package hashdb

import (
	"context"
	"math/big"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	go_test_pg "github.com/olomix/go-test-pg/v2"
	"github.com/stretchr/testify/require"
)

var dbtest = go_test_pg.Pgpool{
	BaseName:   "rhs",
	SchemaFile: "../schema.sql",
	Skip:       false,
}

func TestPgStorage_SaveMiddleNode(t *testing.T) {
	storage := New(dbtest.WithEmpty(t))
	mn := makeMiddleNode(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316",
		"13668806873217811193138343672265398727158334092717678918544074543040898436197",
		"6845643050256962634421298815823256099092239904213746305198440125223303121384")
	created, err := storage.SaveMiddleNode(context.Background(), mn)
	require.NoError(t, err)
	require.True(t, created)
	created, err = storage.SaveMiddleNode(context.Background(), mn)
	require.NoError(t, err)
	require.False(t, created)
}

func TestPgStorage_SaveLeaf(t *testing.T) {
	storage := New(dbtest.WithEmpty(t))
	ln := Leaf(makeHash(t,
		"16938931282012536952003457515784019977456394464750325752202529629073057526316"))
	created, err := storage.SaveLeaf(context.Background(), ln)
	require.NoError(t, err)
	require.True(t, created)
	created, err = storage.SaveLeaf(context.Background(), ln)
	require.NoError(t, err)
	require.False(t, created)
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
	created, err := storage.SaveMiddleNode(ctx, mn)
	require.NoError(t, err)
	require.True(t, created)

	lHash := makeHash(t,
		"13668806873217811193138343672265398727158334092717678918544074543040898436197")
	ln := Leaf(lHash)
	created, err = storage.SaveLeaf(ctx, ln)
	require.NoError(t, err)
	require.True(t, created)

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
