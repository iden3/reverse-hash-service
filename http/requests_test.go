package http

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	"github.com/stretchr/testify/require"
)

func TestRequestsSubmitUnmarshal(t *testing.T) {
	in := `[
{},
{
  "hash": "2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
  "left": "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
  "right": "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
},
{
  "hash": "2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325"
}
]`
	var r nodeSubmitRequest
	err := json.Unmarshal([]byte(in), &r)
	require.NoError(t, err)

	want := nodeSubmitRequest{
		{},
		{
			hash: mkHash(t,
				"16938931282012536952003457515784019977456394464750325752202529629073057526316"),
			left: mkHash(t,
				"13668806873217811193138343672265398727158334092717678918544074543040898436197"),
			right: mkHash(t,
				"6845643050256962634421298815823256099092239904213746305198440125223303121384"),
		},
		{
			hash: mkHash(t,
				"16938931282012536952003457515784019977456394464750325752202529629073057526316"),
		},
	}
	require.Equal(t, want, r)
}

func mkHash(t testing.TB, s string) merkletree.Hash {
	i, ok := new(big.Int).SetString(s, 10)
	require.True(t, ok)
	h := merkletree.NewHashFromBigInt(i)
	return *h
}
