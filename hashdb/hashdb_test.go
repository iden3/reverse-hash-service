package hashdb

import (
	"context"
	"math/big"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	go_test_pg "github.com/olomix/go-test-pg"
	"github.com/stretchr/testify/require"
)

var dbtest = go_test_pg.Pgpool{
	BaseName:   "rhs",
	SchemaFile: "../schema.sql",
	Skip:       false,
}

func TestPgStorage_SaveMiddleNode(t *testing.T) {
	storage := New(dbtest.WithSQLs(t, []string{
		`
INSERT INTO mt_node (hash, lchild, rchild) VALUES
  (
    E'\\x390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410',
    E'\\x1111111111111111111111111111111111111111111111111111111111111111',
    E'\\x0000000000000000000000000000000000000000000000000000000000000000'
  ),
  (
    E'\\x5e0ce1bbf42d77fa28d3e3d67cfd63fd25f04f724cf14ef1365960f0df316e19',
    NULL,
    NULL
  )`,
	}))

	ctx := context.Background()

	mn := makeMiddleNode(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316",
		"13668806873217811193138343672265398727158334092717678918544074543040898436197",
		"6845643050256962634421298815823256099092239904213746305198440125223303121384")
	err := storage.SaveMiddleNode(ctx, mn)
	require.NoError(t, err)
	err = storage.SaveMiddleNode(ctx, mn)
	require.ErrorIs(t, err, ErrAlreadyExists)

	mn = makeMiddleNode(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316",
		"11111111113217811193138343672265398727158334092717678918544074543040898436197",
		"6845643050256962634421298815823256099092239904213746305198440125223303121384")
	err = storage.SaveMiddleNode(ctx, mn)
	require.ErrorIs(t, err, ErrIncorrectHash)

	mn = makeMiddleNode(t,
		"7611690987207287456482922174590148392604233641821586556686773300599336864313",
		"10992852378443248723905195818991694990575654826826443287789374314498732451112",
		"16938931282012536952003457515784019977456394464750325752202529629073057526316")
	err = storage.SaveMiddleNode(ctx, mn)
	require.ErrorIs(t, err, ErrCollision)

	mn = makeMiddleNode(t,
		"11502518614660966970001255759017683381052690921079261831452444371947400268894",
		"9121124718421336894498474736143976849105894580426274368434923283192853493170",
		"7611690987207287456482922174590148392604233641821586556686773300599336864313")
	err = storage.SaveMiddleNode(ctx, mn)
	require.ErrorIs(t, err, ErrLeafUpgraded)
}

func TestPgStorage_SaveLeaf(t *testing.T) {
	storage := New(dbtest.WithEmpty(t))
	ctx := context.Background()

	ln := Leaf(makeHash(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316"))
	err := storage.SaveLeaf(ctx, ln)
	require.NoError(t, err)

	err = storage.SaveLeaf(ctx, ln)
	require.ErrorIs(t, err, ErrAlreadyExists)

	// try to insert leaf node with same hash as existing middle node
	mn := makeMiddleNode(t,
		"11502518614660966970001255759017683381052690921079261831452444371947400268894",
		"9121124718421336894498474736143976849105894580426274368434923283192853493170",
		"7611690987207287456482922174590148392604233641821586556686773300599336864313")
	err = storage.SaveMiddleNode(ctx, mn)
	require.NoError(t, err)
	ln = Leaf(makeHash(t,
		"11502518614660966970001255759017683381052690921079261831452444371947400268894"))
	err = storage.SaveLeaf(ctx, ln)
	require.ErrorIs(t, err, ErrMiddleNodeExists)
}

func TestPgStorage_ByHash(t *testing.T) {
	storage := New(dbtest.WithEmpty(t))

	mHash := makeHash(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316")
	mn := makeMiddleNode(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316",
		"13668806873217811193138343672265398727158334092717678918544074543040898436197",
		"6845643050256962634421298815823256099092239904213746305198440125223303121384")
	ctx := context.Background()
	err := storage.SaveMiddleNode(ctx, mn)
	require.NoError(t, err)

	lHash := makeHash(t,
		"13668806873217811193138343672265398727158334092717678918544074543040898436197")
	ln := Leaf(lHash)
	err = storage.SaveLeaf(ctx, ln)
	require.NoError(t, err)

	mn2, err := storage.ByHash(ctx, mHash)
	require.NoError(t, err)
	require.NotNil(t, mn2)
	mn3, ok := mn2.(MiddleNode)
	require.True(t, ok)
	require.Equal(t, mn, mn3)

	ln2, err := storage.ByHash(ctx, lHash)
	require.NoError(t, err)
	require.NotNil(t, ln2)
	ln3, ok := ln2.(Leaf)
	require.True(t, ok)
	require.Equal(t, ln, ln3)

	missingHash := makeHash(t, "1")
	_, err = storage.ByHash(ctx, missingHash)
	require.EqualError(t, err, ErrDoesNotExists.Error())
}

func TestCalcHash(t *testing.T) {
	testCases := []struct {
		title string
		hash  string
		left  string
		right string
	}{
		{
			title: "both children not 0",
			hash:  "16938931282012536952003457515784019977456394464750325752202529629073057526316",
			left:  "13668806873217811193138343672265398727158334092717678918544074543040898436197",
			right: "6845643050256962634421298815823256099092239904213746305198440125223303121384",
		},
		{
			title: "left child is 0",
			hash:  "387517862079401946799376409801990709903441669470895093924339414901271074750",
			left:  "0",
			right: "16938931282012536952003457515784019977456394464750325752202529629073057526316",
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			mn := makeMiddleNode(t, tc.hash, tc.left, tc.right)

			calcedHash, err := mn.calcHash()
			require.NoError(t, err)
			require.Equal(t, mn.Hash, calcedHash)
		})
	}
}

func makeMiddleNode(t testing.TB, h, l, r string) MiddleNode {
	return MiddleNode{
		Hash:  makeHash(t, h),
		Left:  makeHash(t, l),
		Right: makeHash(t, r)}
}

func makeHash(t testing.TB, s string) merkletree.Hash {
	i, ok := new(big.Int).SetString(s, 10)
	require.True(t, ok)
	h := merkletree.NewHashFromBigInt(i)
	return *h
}
