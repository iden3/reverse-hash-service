package http

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	"github.com/stretchr/testify/require"
)

func TestNodeResponse_MarshalJSON(t *testing.T) {
	leafNode := mkNode(t,
		"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
		[]string{
			"037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"0100000000000000000000000000000000000000000000000000000000000000",
		})

	data, err := json.Marshal(nodeResponse{leafNode, statusOK})
	require.NoError(t, err)
	want := `{
  "status":"OK",
  "node":{
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "children": [
      "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
      "0000000000000000000000000000000000000000000000000000000000000000",
      "0100000000000000000000000000000000000000000000000000000000000000"
    ]
  }
}`
	require.JSONEq(t, want, string(data))

	middleNode := mkNode(t,
		"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
		[]string{
			"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f",
		})
	data, err = json.Marshal(nodeResponse{middleNode, statusError})
	require.NoError(t, err)
	want = `{
  "status":"error",
  "node":{
    "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
    "children": [
      "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
      "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
    ]
  }
}`
	require.JSONEq(t, want, string(data))
}

func hashFromHex(t testing.TB, in string) merkletree.Hash {
	var h merkletree.Hash
	_, err := hex.Decode(h[:], []byte(in))
	require.NoError(t, err)
	return h
}
