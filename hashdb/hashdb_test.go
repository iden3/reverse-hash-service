package hashdb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	merkletree "github.com/iden3/go-merkletree-sql"
	"github.com/iden3/go-merkletree-sql/db/memory"
	"github.com/jackc/pgtype"
	go_test_pg "github.com/olomix/go-test-pg"
	"github.com/stretchr/testify/require"
)

var dbtest = go_test_pg.Pgpool{
	BaseName:   "rhs",
	SchemaFile: "../schema.sql",
	Skip:       false,
}

func TestPgStorage_ByHash(t *testing.T) {
	storage := New(dbtest.WithEmpty(t))

	n1 := makeNode(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316",
		[]string{
			"13668806873217811193138343672265398727158334092717678918544074543040898436197",
			"6845643050256962634421298815823256099092239904213746305198440125223303121384",
		},
	)
	n2 := makeNodeHex(t,
		"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
		[]string{
			"037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"0100000000000000000000000000000000000000000000000000000000000000",
		},
	)

	ctx := context.Background()
	err := storage.SaveNodes(ctx, []Node{n1, n2})
	require.NoError(t, err)

	n3, err := storage.ByHash(ctx, n1.Hash)
	require.NoError(t, err)
	require.Equal(t, n1, n3)

	n4, err := storage.ByHash(ctx, n2.Hash)
	require.NoError(t, err)
	require.Equal(t, n2, n4)

	missingHash := hashFromIntString(t, "1")
	_, err = storage.ByHash(ctx, missingHash)
	require.EqualError(t, err, ErrDoesNotExists.Error())
}

func TestHashChildren(t *testing.T) {
	testCases := []struct {
		title    string
		hash     string
		children []string
	}{
		{
			title: "both children not 0",
			hash:  "16938931282012536952003457515784019977456394464750325752202529629073057526316",
			children: []string{
				"13668806873217811193138343672265398727158334092717678918544074543040898436197",
				"6845643050256962634421298815823256099092239904213746305198440125223303121384",
			},
		},
		{
			title: "left child is 0",
			hash:  "387517862079401946799376409801990709903441669470895093924339414901271074750",
			children: []string{
				"0",
				"16938931282012536952003457515784019977456394464750325752202529629073057526316",
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			node := makeNode(t, tc.hash, tc.children)
			calcedHash, err := node.hashChildren()
			require.NoError(t, err)
			require.Equal(t, node.Hash, calcedHash)
		})
	}
}

func makeNode(t testing.TB, h string, children []string) Node {
	node := Node{Hash: hashFromIntString(t, h)}
	for _, i := range children {
		node.Children = append(node.Children, hashFromIntString(t, i))
	}
	return node
}

func makeNodeHex(t testing.TB, h string, children []string) Node {
	var node = Node{Children: make([]merkletree.Hash, len(children))}
	require.Len(t, h, len(node.Hash)*2)
	n, err := hex.Decode(node.Hash[:], []byte(h))
	require.NoError(t, err)
	require.Equal(t, len(node.Hash), n)

	for i := range children {
		require.Len(t, children[i], len(node.Children[i])*2)
		n, err := hex.Decode(node.Children[i][:], []byte(children[i]))
		require.NoError(t, err)
		require.Equal(t, len(node.Children[i]), n)
	}

	return node
}

func hashFromIntString(t testing.TB, s string) merkletree.Hash {
	i, ok := new(big.Int).SetString(s, 10)
	require.True(t, ok)
	h := merkletree.NewHashFromBigInt(i)
	return *h
}

func TestNode_UnmarshalJSON(t *testing.T) {
	testCases := []struct {
		title   string
		in      string
		want    Node
		wantErr string
	}{
		{
			title: "middle node",
			in: `{
  "hash": "2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
  "children": [
    "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
  ]
}`,
			want: makeNode(t,
				"16938931282012536952003457515784019977456394464750325752202529629073057526316",
				[]string{
					"13668806873217811193138343672265398727158334092717678918544074543040898436197",
					"6845643050256962634421298815823256099092239904213746305198440125223303121384",
				},
			),
		},
		{
			title: "leaf node",
			in: `{
  "hash": "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
  "children": [
    "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
    "0000000000000000000000000000000000000000000000000000000000000000",
    "0100000000000000000000000000000000000000000000000000000000000000"
  ]
}`,
			want: makeNode(t,
				"13668806873217811193138343672265398727158334092717678918544074543040898436197",
				[]string{
					"13260572831089785859",
					"0",
					"1",
				},
			),
		},
		{
			title: "incorrect hash node",
			in: `{
  "hash": "668c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
  "children": [
    "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
    "0000000000000000000000000000000000000000000000000000000000000000",
    "0100000000000000000000000000000000000000000000000000000000000000"
  ]
}`,
			wantErr: "node hash is not correct",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			var node Node
			err := json.Unmarshal([]byte(tc.in), &node)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.want, node)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

func TestNode_MarshalJSON(t *testing.T) {
	want := `{
  "hash": "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
  "children": [
    "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
    "0000000000000000000000000000000000000000000000000000000000000000",
    "0100000000000000000000000000000000000000000000000000000000000000"
  ]
}`
	node := makeNode(t,
		"13668806873217811193138343672265398727158334092717678918544074543040898436197",
		[]string{
			"13260572831089785859",
			"0",
			"1",
		},
	)
	bytes, err := json.Marshal(node)
	require.NoError(t, err)
	require.JSONEq(t, want, string(bytes))
}

func TestMkInsertNodesSQL(t *testing.T) {
	nodeLeaf := makeNodeHex(t,
		"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
		[]string{
			"037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"0100000000000000000000000000000000000000000000000000000000000000",
		})
	var leafNodeHash pgtype.Bytea
	err := leafNodeHash.Set(nodeLeaf.Hash[:])
	require.NoError(t, err)
	var leafNodeChildren pgtype.ByteaArray
	err = leafNodeChildren.Set([][]byte{
		nodeLeaf.Children[0][:],
		nodeLeaf.Children[1][:],
		nodeLeaf.Children[2][:]})
	require.NoError(t, err)

	middleNode := makeNodeHex(t,
		"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
		[]string{
			"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f",
		})
	var middleNodeHash pgtype.Bytea
	err = middleNodeHash.Set(middleNode.Hash[:])
	require.NoError(t, err)
	var middleNodeChildren pgtype.ByteaArray
	err = middleNodeChildren.Set([][]byte{
		middleNode.Children[0][:],
		middleNode.Children[1][:]})
	require.NoError(t, err)

	query, params, err := mkInsertNodesSQL([]Node{nodeLeaf, middleNode})
	require.NoError(t, err)
	wantQuery := `
INSERT INTO "mt_node" (hash, children)
VALUES ($1,$2),($3,$4)
ON CONFLICT DO NOTHING`
	require.Equal(t, wantQuery, query)
	wantParams := []interface{}{
		leafNodeHash, leafNodeChildren, middleNodeHash, middleNodeChildren}
	require.Equal(t, wantParams, params)
}

// insert bunch of nodes, greater than insertNodeChunkSize, to test
// chunked database insertion
func TestChunkedNodeSaving(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	mtStorage := memory.NewMemoryStorage()
	const mtDepth = 40
	ctx := context.Background()
	mt, err := merkletree.NewMerkleTree(ctx, mtStorage, mtDepth)
	require.NoError(t, err)

	for i := int64(1); i < insertNodeChunkSize*1.5; i++ {
		err = mt.Add(ctx, big.NewInt(i), big.NewInt(10))
		require.NoError(t, err)
	}

	var nodes []Node
	oneHash := hashFromIntString(t, "1")

	err = mt.Walk(ctx, nil, func(n *merkletree.Node) {
		hash, err := n.Key()
		require.NoError(t, err)
		n2 := Node{Hash: *hash}
		switch n.Type {
		case merkletree.NodeTypeMiddle:
			n2.Children = append(n2.Children, *n.ChildL, *n.ChildR)
		case merkletree.NodeTypeLeaf:
			n2.Children = append(n2.Children, *n.Entry[0], *n.Entry[1], oneHash)
		default:
			t.Fatalf("unexpected node type: %v", n.Type)
		}
		nodes = append(nodes, n2)
	})
	require.NoError(t, err)

	require.True(t, len(nodes) > insertNodeChunkSize)

	storage := New(dbtest.WithEmpty(t))
	err = storage.SaveNodes(ctx, nodes)
	require.NoError(t, err)

	for _, n := range nodes {
		node, err := storage.ByHash(ctx, n.Hash)
		require.NoError(t, err)
		require.Equal(t, n, node)
	}
}
