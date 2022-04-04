package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	stderr "errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/iden3/go-iden3-crypto/poseidon"
	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/go-merkletree-sql/db/memory"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestZeroTree(t *testing.T) {
	mtStorage := memory.NewMemoryStorage()
	ctx := context.Background()
	const mtDepth = 40
	mt, err := merkletree.NewMerkleTree(ctx, mtStorage, mtDepth)
	require.NoError(t, err)
	root := mt.Root()
	t.Log(root.Hex(), root.BigInt().Text(10))
}

func TestProof(t *testing.T) {
	//nolint:gocritic
	//  revNonceKey2 := new(merkletree.Hash)
	//  revNonceKey2[0] = 0b00011111
	//  revNonceKeyInt2 := new(big.Int).
	//  	SetBytes(merkletree.SwapEndianness(revNonceKey2[:]))
	//  t.Logf("rn2 = %v", revNonceKeyInt2)

	storage := hashdb.New(dbtest.WithEmpty(t))
	router := setupRouter(storage)
	ts := httptest.NewServer(router)
	defer ts.Close()

	revNonces := []uint64{
		5577006791947779410,  // 19817761...  0 1 0 0 1 0 1 0
		8674665223082153551,  // 68456430...  1 1 1 1 0 0 1 0
		8674665223082147919,  // a node is very close to 8674665223082153551 — to generate zero siblings
		15352856648520921629, // 86798249...  1 0 1 1 1 0 0 0
		13260572831089785859, // 13668806...  1 1 0 0 0 0 0 0
		3916589616287113937,  // 50401982...  1 0 0 0 1 0 1 1
		6334824724549167320,  // 38589333...  0 0 0 1 1 0 1 1
		9828766684487745566,  // 55091915...  0 1 1 1 1 0 0 0
		10667007354186551956, // 10419680...  0 0 1 0 1 0 0 1
		894385949183117216,   // 13133085...  0 0 0 0 0 1 0 1
		11998794077335055257, // 14875578...  1 0 0 1 1 0 0 1
	}

	bigMerkleTree := buildTree(t, revNonces)
	bigMerkleTreeRoot := bigMerkleTree.Root()
	//nolint:gocritic
	// drawDotTree(bigMerkleTree)
	saveTreeToRHS(t, router, bigMerkleTree)
	drawTree(t, bigMerkleTree)

	oneNodeMerkleTree := buildTree(t, []uint64{5577006791947779410})
	saveTreeToRHS(t, router, oneNodeMerkleTree)
	oneNodeMerkleTreeRoot := oneNodeMerkleTree.Root()

	t.Run("Test save state", func(t *testing.T) {
		state := saveIdenStateToRHS(t, router, bigMerkleTree)

		revTreeRoot, err := getRevTreeRoot(ts.URL, state)
		require.NoError(t, err)

		revTreeRootExpected := *bigMerkleTreeRoot
		require.Equal(t, revTreeRootExpected, revTreeRoot)
	})

	testCases := []struct {
		title       string
		revNonce    uint64
		revTreeRoot merkletree.Hash
		wantProof   Proof
		wantErr     string
	}{
		{
			title:       "regular node",
			revNonce:    10667007354186551956,
			revTreeRoot: *bigMerkleTreeRoot,
			wantProof: Proof{
				Existence: true,
				Siblings: []merkletree.Hash{
					mkHash("74321998e281c0a89dbcce55a6cec0e366536e2697ea40efaf036ecba751ed03"),
					mkHash("ff11b8bf1d13e28e86e249d2acdba0bd9c0fe4a5f56ad4236b09185bde81c316"),
					mkHash("db5eb80f6b60b4e23714d4d00f178ba62fbdb4f0294675f51ac99aa24e600827"),
				},
				NodeAux: nil,
			},
		},
		{
			title:       "a node with zero siblings",
			revNonce:    8674665223082147919,
			revTreeRoot: *bigMerkleTreeRoot,
			wantProof: Proof{
				Existence: true,
				Siblings: []merkletree.Hash{
					mkHash("b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14"),
					mkHash("28e5cdd29d9ad96cc214c654ca8e2f4fa5576bc132e172519804a58ee4bb4d18"),
					mkHash("658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"),
				},
				NodeAux: nil,
			},
		},
		{
			title: "un-existence with aux node",
			//nolint:gocritic
			revNonce:    5, // revNonceKey[0] = 0b00000101
			revTreeRoot: *bigMerkleTreeRoot,
			wantProof: Proof{
				Existence: false,
				Siblings: []merkletree.Hash{
					mkHash("b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14"),
					mkHash("c9719432e3d8bf360d0f2de456c5321c51295895c9330b0588552580765cd929"),
					mkHash("c0e8bf477403a8161cc2153597ff7791f67e6cfde6a96ca2748292662ec78d0a"),
				},
				NodeAux: &NodeAux{
					Key:   mkHashFromInt(15352856648520921629),
					Value: mkHashFromInt(0),
				},
			},
		},
		{
			title: "test un-existence without aux node",
			//nolint:gocritic
			revNonce:    31, // revNonceKey[0] = 0b00011111
			revTreeRoot: *bigMerkleTreeRoot,
			wantProof: Proof{
				Existence: false,
				Siblings: []merkletree.Hash{
					mkHash("b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14"),
					mkHash("28e5cdd29d9ad96cc214c654ca8e2f4fa5576bc132e172519804a58ee4bb4d18"),
					mkHash("658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"),
					mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
					mkHash("5aa678402ef2cd5102de99722a6923183461b93f705a9d0aaaaff6a131a83504"),
				},
				NodeAux: nil,
			},
		},
		{
			title:       "test node does not exists",
			revNonce:    31,
			revTreeRoot: mkHash("1234567812345678123456781234567812345678123456781234567812345678"),
			wantErr:     "node not found",
		},
		{
			title:       "test zero tree root",
			revNonce:    31,
			revTreeRoot: mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
			wantProof: Proof{
				Existence: false,
				Siblings:  nil,
				NodeAux:   nil,
			},
		},
		{
			title:       "existence of one only node in a tree",
			revNonce:    5577006791947779410,
			revTreeRoot: *oneNodeMerkleTreeRoot,
			wantProof: Proof{
				Existence: true,
				Siblings:  nil,
				NodeAux:   nil,
			},
		},
		{
			title:       "un-existence of one only node in a tree",
			revNonce:    10667007354186551956,
			revTreeRoot: *oneNodeMerkleTreeRoot,
			wantProof: Proof{
				Existence: false,
				Siblings:  nil,
				NodeAux: &NodeAux{
					Key:   mkHashFromInt(5577006791947779410),
					Value: mkHashFromInt(0),
				},
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			revNonceKeyInt := new(big.Int).SetUint64(tc.revNonce)
			revNonceKey := merkletree.NewHashFromBigInt(revNonceKeyInt)
			revNonceValueInt := big.NewInt(0)

			proof, err := generateProof(ts.URL, tc.revTreeRoot, *revNonceKey)
			if tc.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantProof, proof)

				rootHash, err := proof.Root(revNonceKeyInt, revNonceValueInt)
				require.NoError(t, err)
				require.Equal(t, tc.revTreeRoot, rootHash)

				//nolint:gocritic
				// logProof(t, proof)
			} else {
				require.EqualError(t, err, tc.wantErr)
			}
		})
	}
}

//nolint:deadcode,unused //reason:need to generate
func logProof(t testing.TB, proof Proof) {
	proofBytes, err := json.Marshal(proof)
	require.NoError(t, err)
	t.Log(string(proofBytes))
}

type NodeAux struct {
	Key   merkletree.Hash
	Value merkletree.Hash
}

func (n NodeAux) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"key":   n.Key.Hex(),
		"value": n.Value.Hex(),
	})
}

type Proof struct {
	Existence bool              `json:"existence"`
	Siblings  []merkletree.Hash `json:"siblings"`
	NodeAux   *NodeAux          `json:"aux_node"`
}

func (p *Proof) UnmarshalJSON(data []byte) error {
	p.Siblings = nil
	p.NodeAux = nil

	var obj map[string]interface{}
	err := json.Unmarshal(data, &obj)
	if err != nil {
		return err
	}

	exI, ok := obj["existence"]
	if !ok {
		return errors.New("existence key not found")
	}
	p.Existence, ok = exI.(bool)
	if !ok {
		return errors.New("incorrect type of existence key")
	}

	sibI, ok := obj["siblings"]
	if !ok || sibI == nil {
		p.Siblings = nil
	} else {
		sibL, ok := sibI.([]interface{})
		if !ok {
			return errors.Errorf("incorrect type of siblings key: %T", sibI)
		}
		p.Siblings = make([]merkletree.Hash, len(sibL))
		for i, s := range sibL {
			sS, ok := s.(string)
			if !ok {
				return errors.Errorf("sibling #%v is not string", i)
			}
			p.Siblings[i], err = unmarshalHex(sS)
			if err != nil {
				return errors.Errorf("errors unmarshal sibling #%v: %v", i, err)
			}
		}
	}

	anI, ok := obj["aux_node"]
	if !ok || anI == nil {
		p.NodeAux = nil
		return nil
	}

	anI2, ok := anI.(map[string]interface{})
	if !ok {
		return errors.New("aux_node has incorrect format")
	}

	p.NodeAux = new(NodeAux)

	keyI, ok := anI2["key"]
	if !ok {
		return errors.New("aux_node has not key")
	}

	keyS, ok := keyI.(string)
	if !ok {
		return errors.New("aux_node key is not a string")
	}

	hashBytes, err := hex.DecodeString(keyS)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(hashBytes) != len(p.NodeAux.Key) {
		return errors.New("incorrect aux_node key length")
	}

	copy(p.NodeAux.Key[:], hashBytes)

	valueI, ok := anI2["value"]
	if !ok {
		return errors.New("aux_node has not value")
	}

	valueS, ok := valueI.(string)
	if !ok {
		return errors.New("aux_node value is not a string")
	}

	hashBytes, err = hex.DecodeString(valueS)
	if err != nil {
		return errors.WithStack(err)
	}
	if len(hashBytes) != len(p.NodeAux.Value) {
		return errors.New("incorrect aux_node value length")
	}

	copy(p.NodeAux.Value[:], hashBytes)

	return nil
}

func unmarshalHex(in string) (merkletree.Hash, error) {
	var h merkletree.Hash
	data, err := hex.DecodeString(in)
	if err != nil {
		return h, err
	}
	if len(data) != len(h) {
		return h, errors.New("incorrect length")
	}
	copy(h[:], data)
	return h, nil
}

func (p Proof) MarshalJSON() ([]byte, error) {
	siblings := make([]string, len(p.Siblings))
	for i := range p.Siblings {
		siblings[i] = p.Siblings[i].Hex()
	}
	obj := map[string]interface{}{
		"existence": p.Existence,
		"siblings":  siblings}
	if p.NodeAux != nil {
		obj["aux_node"] = p.NodeAux
	}
	return json.Marshal(obj)
}

type NodeType byte

const (
	NodeTypeUnknown NodeType = iota
	NodeTypeMiddle  NodeType = iota
	NodeTypeLeaf    NodeType = iota
	NodeTypeState   NodeType = iota
)

var ErrNodeNotFound = stderr.New("node not found")

func getRevTreeRoot(rhsURL string,
	state merkletree.Hash) (merkletree.Hash, error) {

	stateNode, err := getNodeFromRHS(rhsURL, state)
	if err != nil {
		return merkletree.HashZero, err
	}

	if len(stateNode.Children) != 3 {
		return merkletree.HashZero, errors.New(
			"state hash does not looks like a state node: " +
				"number of children expected to be three")
	}

	return stateNode.Children[1], nil
}

func generateProof(rhsURL string, treeRoot merkletree.Hash,
	key merkletree.Hash) (Proof, error) {
	nextKey := treeRoot
	var p Proof
	for depth := uint(0); depth < uint(len(key)*8); depth++ {
		if nextKey == merkletree.HashZero {
			return p, nil
		}
		n, err := getNodeFromRHS(rhsURL, nextKey)
		if err != nil {
			return p, err
		}
		switch nt := nodeType(n); nt {
		case NodeTypeLeaf:
			if key == n.Children[0] {
				p.Existence = true
				return p, nil
			}
			// We found a leaf whose entry didn't match hIndex
			p.NodeAux = &NodeAux{Key: n.Children[0], Value: n.Children[1]}
			return p, nil
		case NodeTypeMiddle:
			var siblingKey merkletree.Hash
			if merkletree.TestBit(key[:], depth) {
				nextKey = n.Children[1]
				siblingKey = n.Children[0]
			} else {
				nextKey = n.Children[0]
				siblingKey = n.Children[1]
			}
			p.Siblings = append(p.Siblings, siblingKey)
		default:
			return p, errors.Errorf(
				"found unexpected node type in tree (%v): %v",
				nt, n.Hash.Hex())
		}
	}

	return p, errors.New("tree depth is too high")
}

func nodeType(node hashdb.Node) NodeType {
	if len(node.Children) == 2 {
		return NodeTypeMiddle
	}

	hashOne := merkletree.NewHashFromBigInt(big.NewInt(1))
	if len(node.Children) == 3 && node.Children[2] == *hashOne {
		return NodeTypeLeaf
	}

	if len(node.Children) == 3 {
		return NodeTypeState
	}

	return NodeTypeUnknown
}

func saveIdenStateToRHS(t testing.TB, httpRouter http.Handler,
	merkleTree *merkletree.MerkleTree) merkletree.Hash {

	revTreeRoot := merkleTree.Root()
	state, err := poseidon.Hash([]*big.Int{big.NewInt(0), revTreeRoot.BigInt(),
		big.NewInt(0)})
	require.NoError(t, err)
	stateHash := merkletree.NewHashFromBigInt(state)

	req := nodeSubmitRequest{
		{
			Hash: *stateHash,
			Children: []merkletree.Hash{
				merkletree.HashZero, *revTreeRoot, merkletree.HashZero},
		},
	}
	submitNodesToRHS(t, httpRouter, req)
	return *stateHash
}

func saveTreeToRHS(t testing.TB,
	httpRouter http.Handler, merkleTree *merkletree.MerkleTree) {
	ctx := context.Background()
	var req nodeSubmitRequest
	hashOne := merkletree.NewHashFromBigInt(big.NewInt(1))
	err := merkleTree.Walk(ctx, nil, func(node *merkletree.Node) {
		nodeKey, err := node.Key()
		require.NoError(t, err)
		switch node.Type {
		case merkletree.NodeTypeMiddle:
			req = append(req, hashdb.Node{
				Hash:     *nodeKey,
				Children: []merkletree.Hash{*node.ChildL, *node.ChildR},
			})
		case merkletree.NodeTypeLeaf:
			req = append(req, hashdb.Node{
				Hash: *nodeKey,
				Children: []merkletree.Hash{
					*node.Entry[0], *node.Entry[1], *hashOne},
			})
		case merkletree.NodeTypeEmpty:
			// do not save zero nodes
		default:
			require.Failf(t, "unexpected node type", "unexpected node type: %v",
				node.Type)
		}
	})
	require.NoError(t, err)

	submitNodesToRHS(t, httpRouter, req)
}

func drawTree(t testing.TB, merkleTree *merkletree.MerkleTree) {
	ctx := context.Background()
	var req nodeSubmitRequest
	hashOne := merkletree.NewHashFromBigInt(big.NewInt(1))
	err := merkleTree.Walk(ctx, nil, func(node *merkletree.Node) {
		nodeKey, err := node.Key()
		require.NoError(t, err)
		switch node.Type {
		case merkletree.NodeTypeMiddle:
			req = append(req, hashdb.Node{
				Hash:     *nodeKey,
				Children: []merkletree.Hash{*node.ChildL, *node.ChildR},
			})
		case merkletree.NodeTypeLeaf:
			req = append(req, hashdb.Node{
				Hash: *nodeKey,
				Children: []merkletree.Hash{
					*node.Entry[0], *node.Entry[1], *hashOne},
			})
		case merkletree.NodeTypeEmpty:
			// do not save zero nodes
		default:
			require.Failf(t, "unexpected node type", "unexpected node type: %v",
				node.Type)
		}
	})
	require.NoError(t, err)
	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)

	t.Log(string(reqBytes))
}

func submitNodesToRHS(t testing.TB,
	httpRouter http.Handler, req nodeSubmitRequest) {

	reqBytes, err := json.Marshal(req)
	require.NoError(t, err)
	bodyReader := bytes.NewReader(reqBytes)
	httpReq, err := http.NewRequest(http.MethodPost, "/node", bodyReader)
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	httpRouter.ServeHTTP(rr, httpReq)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.JSONEq(t, `{"status":"OK"}`, rr.Body.String(), rr.Body.String())
}

func getNodeFromRHS(rhsURL string, hash merkletree.Hash) (hashdb.Node, error) {
	rhsURL = strings.TrimSuffix(rhsURL, "/")
	rhsURL += "/node/" + hash.Hex()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(
		ctx, http.MethodGet, rhsURL, http.NoBody)
	if err != nil {
		return hashdb.Node{}, errors.WithStack(err)
	}

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return hashdb.Node{}, errors.WithStack(err)
	}

	defer httpResp.Body.Close()
	if httpResp.StatusCode == http.StatusNotFound {
		var resp map[string]interface{}
		dec := json.NewDecoder(httpResp.Body)
		err := dec.Decode(&resp)
		if err != nil {
			return hashdb.Node{}, errors.WithStack(err)
		}
		if resp["status"] == "not found" {
			return hashdb.Node{}, ErrNodeNotFound
		} else {
			return hashdb.Node{}, errors.New("unexpected response")
		}
	} else if httpResp.StatusCode != http.StatusOK {
		return hashdb.Node{}, errors.Errorf("unexpected response: %v",
			httpResp.StatusCode)
	}

	var nodeResp nodeResponse
	dec := json.NewDecoder(httpResp.Body)
	err = dec.Decode(&nodeResp)
	if err != nil {
		return hashdb.Node{}, errors.WithStack(err)
	}

	// begin debug
	// x, err := json.Marshal(nodeResp.Node)
	// if err != nil {
	// 	panic(err)
	// }
	// log.Printf(string(x))
	// end debug

	return nodeResp.Node, nil
}

func buildTree(t testing.TB, revNonces []uint64) *merkletree.MerkleTree {
	mtStorage := memory.NewMemoryStorage()
	ctx := context.Background()
	const mtDepth = 40
	mt, err := merkletree.NewMerkleTree(ctx, mtStorage, mtDepth)
	require.NoError(t, err)

	for _, revNonce := range revNonces {
		key := new(big.Int).SetUint64(revNonce)
		value := big.NewInt(0)

		err = mt.Add(ctx, key, value)
		require.NoError(t, err)
	}

	return mt
}

func (p Proof) Root(k, v *big.Int) (merkletree.Hash, error) {
	kHash := merkletree.NewHashFromBigInt(k)
	vHash := merkletree.NewHashFromBigInt(v)
	var midKey merkletree.Hash
	if p.Existence {
		leafKey, err := merkletree.LeafKey(kHash, vHash)
		if err != nil {
			return midKey, err
		}
		midKey = *leafKey
	} else {
		if p.NodeAux == nil {
			midKey = merkletree.HashZero
		} else {
			if bytes.Equal(kHash[:], p.NodeAux.Key[:]) {
				return midKey, errors.New(
					"Non-existence proof being checked against hIndex equal " +
						"to nodeAux")
			}
			leafKey, err := merkletree.LeafKey(&p.NodeAux.Key, &p.NodeAux.Value)
			if err != nil {
				return midKey, err
			}
			midKey = *leafKey
		}
	}

	for lvl := len(p.Siblings) - 1; lvl >= 0; lvl-- {
		if merkletree.TestBit(kHash[:], uint(lvl)) {
			mKey, err := merkletree.NewNodeMiddle(
				&p.Siblings[lvl], &midKey).Key()
			if err != nil {
				return midKey, err
			}
			midKey = *mKey
		} else {
			mKey, err := merkletree.NewNodeMiddle(
				&midKey, &p.Siblings[lvl]).Key()
			if err != nil {
				return midKey, err
			}
			midKey = *mKey
		}
	}
	return midKey, nil
}

func TestFindCloseNode(t *testing.T) {
	// 		8674665223082153551,  // 68456430...  1 1 1 1 0 0 1 0
	intBytes := merkletree.SwapEndianness(
		new(big.Int).SetUint64(8674665223082153551).Bytes())
	t.Logf("%08b", intBytes[:5])

	intBytes[1] = 0
	i := new(big.Int).SetBytes(merkletree.SwapEndianness(intBytes))
	t.Log(i)
}

//nolint:deadcode,unused //reason: need to generate
func drawDotTree(mt *merkletree.MerkleTree) {
	fmt.Fprint(os.Stderr, `digraph hierarchy {
node [fontname=Monospace,fontsize=10,shape=box]
`)
	cnt := 0
	var errIn error
	err := mt.Walk(context.Background(), nil, func(n *merkletree.Node) {
		k, err := n.Key()
		if err != nil {
			errIn = err
		}
		switch n.Type {
		case merkletree.NodeTypeEmpty:
		case merkletree.NodeTypeLeaf:
			fmt.Fprintf(os.Stderr,
				"\"%[1]v\" [style=filled,label=\"%[1]v\\n%[2]v\"];\n",
				k.Hex(), n.Entry[0].BigInt().Text(10))
		case merkletree.NodeTypeMiddle:
			lr := [2]string{n.ChildL.Hex(), n.ChildR.Hex()}
			emptyNodes := ""
			for i := range lr {
				if lr[i] == "0000000000000000000000000000000000000000000000000000000000000000" {
					lr[i] = fmt.Sprintf("empty%v", cnt)
					emptyNodes += fmt.Sprintf(
						"\"%v\" [style=dashed,label=0];\n", lr[i])
					cnt++
				}
			}
			fmt.Fprintf(os.Stderr, "\"%v\" -> {\"%v\" \"%v\"}\n", k.Hex(),
				lr[0], lr[1])
			fmt.Fprint(os.Stderr, emptyNodes)
		default:
		}
	})
	fmt.Fprint(os.Stderr, "}\n")
	if errIn != nil {
		panic(errIn)
	}
	if err != nil {
		panic(err)
	}
}

func TestProof_Unmarshal(t *testing.T) {
	testCases := []struct {
		title string
		in    string
		want  Proof
	}{
		{
			title: "OK",
			in: `{
"existence": true,
"siblings": null}`,
			want: Proof{Existence: true},
		},
		{
			title: "only existence",
			in:    `{"existence": true}`,
			want:  Proof{Existence: true},
		},
		{
			title: "null siblings",
			in: `{
  "existence": true,
  "siblings": null
}`,
			want: Proof{Existence: true},
		},
		{
			title: "empty siblings",
			in: `{
  "existence": true,
  "siblings": []
}`,
			want: Proof{Existence: true, Siblings: make([]merkletree.Hash, 0)},
		},
		{
			title: "with siblings",
			in: `{
  "existence": true,
  "siblings": [
    "b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14",
    "74321998e281c0a89dbcce55a6cec0e366536e2697ea40efaf036ecba751ed03"
  ]
}`,
			want: Proof{
				Existence: true,
				Siblings: []merkletree.Hash{
					mkHash("b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14"),
					mkHash("74321998e281c0a89dbcce55a6cec0e366536e2697ea40efaf036ecba751ed03"),
				}},
		},
		{
			title: "with aux_node",
			in: `{
  "existence": true,
  "siblings": [
    "b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14",
    "74321998e281c0a89dbcce55a6cec0e366536e2697ea40efaf036ecba751ed03"
  ],
  "aux_node": {
    "key":   "94d2c422acd20894000000000000000000000000000000000000000000000000",
    "value": "0000000000000000000000000000000000000000000000000000000000000000"
  }
}`,
			want: Proof{
				Existence: true,
				Siblings: []merkletree.Hash{
					mkHash("b2f5a640931d3815375be1e9a00ee4da175d3eb9520ef0715f484b11a75f2a14"),
					mkHash("74321998e281c0a89dbcce55a6cec0e366536e2697ea40efaf036ecba751ed03"),
				},
				NodeAux: &NodeAux{
					Key:   mkHash("94d2c422acd20894000000000000000000000000000000000000000000000000"),
					Value: mkHash("0000000000000000000000000000000000000000000000000000000000000000"),
				},
			},
		},
	}
	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			var p Proof
			err := json.Unmarshal([]byte(tc.in), &p)
			require.NoError(t, err)
			require.Equal(t, tc.want, p)
		})
	}
}

func mkHash(in string) merkletree.Hash {
	data, err := hex.DecodeString(in)
	if err != nil {
		panic(err)
	}
	var h merkletree.Hash
	if len(data) != len(h) {
		panic(len(data))
	}
	copy(h[:], data)
	return h
}

func mkHashFromInt(in uint64) merkletree.Hash {
	i := new(big.Int).SetUint64(in)
	hashBytes := merkletree.SwapEndianness(i.Bytes())
	var h merkletree.Hash
	copy(h[:], hashBytes)
	return h
}
