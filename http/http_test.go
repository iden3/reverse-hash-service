package http

import (
	"context"
	stderr "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	go_test_pg "github.com/olomix/go-test-pg/v2"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

var dbtest = go_test_pg.Pgpool{
	BaseName:   "rhs",
	SchemaFile: "../schema.sql",
	Skip:       false,
}

type nodesStorageMock struct {
	nodes        map[merkletree.Hash]hashdb.Node
	byHashErrors map[merkletree.Hash]error
}

func (n *nodesStorageMock) SaveMiddleNode(_ context.Context,
	_ hashdb.MiddleNode) error {
	panic("not implemented")
}

func (n *nodesStorageMock) SaveLeaf(_ context.Context, _ hashdb.Leaf) error {
	panic("not implemented")
}

func (n *nodesStorageMock) ByHash(_ context.Context,
	hash merkletree.Hash) (hashdb.Node, error) {

	err, ok := n.byHashErrors[hash]
	if ok {
		return nil, err
	}

	node, ok := n.nodes[hash]
	if !ok {
		return nil, errors.WithStack(hashdb.ErrDoesNotExists)
	}
	return node, nil
}

func TestGetNodeHandler(t *testing.T) {
	ng := nodesStorageMock{
		nodes: map[merkletree.Hash]hashdb.Node{
			hashFromHex(t,
				"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325"): hashdb.MiddleNode{
				Hash: hashFromHex(t,
					"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325"),
				Left: hashFromHex(t,
					"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"),
				Right: hashFromHex(t,
					"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"),
			},
			hashFromHex(t,
				"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"): hashdb.Leaf(
				hashFromHex(t,
					"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e")),
		},
		byHashErrors: map[merkletree.Hash]error{
			hashFromHex(t,
				"11111111114ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"): stderr.New("some internal error"),
		},
	}
	router := setupRouter(&ng)

	testCases := []struct {
		title    string
		req      string
		wantCode int
		wantBody string
	}{
		{
			title:    "Get MiddleNode",
			req:      "/node/2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
			wantCode: http.StatusOK,
			wantBody: `{
  "status":"OK",
  "node":{
    "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
    "left":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
  }
}`,
		},
		{
			title:    "Get Leaf",
			req:      "/node/658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusOK,
			wantBody: `{
  "status":"OK",
  "node":{
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"
  }
}`,
		},
		{
			title:    "Missing Node",
			req:      "/node/00000000004ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusNotFound,
			wantBody: `{"status":"not found"}`,
		},
		{
			title:    "Internal error",
			req:      "/node/11111111114ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusInternalServerError,
			wantBody: `{"error":"some internal error","status":"error"}`,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, tc.req, nil)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, tc.wantCode, rr.Code)
			require.JSONEq(t, tc.wantBody, rr.Body.String())
		})
	}
}

// The order of test cases is important. Do not run sub-tests in parallel.
func TestGetNodeSubmitHandler(t *testing.T) {
	storage := hashdb.New(dbtest.WithEmpty(t))
	router := setupRouter(storage)

	testCases := []struct {
		title    string
		req      string
		method   string
		body     string
		wantCode int
		wantBody string

		wantMiddleNodes map[merkletree.Hash]hashdb.MiddleNode
		wantLeafs       map[hashdb.Leaf]struct{}
	}{
		{
			title:  "save few nodes (some with errors)",
			req:    "/node",
			method: http.MethodPost,
			body: `[
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","left":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e","right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"},
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","left":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e","right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"},
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","left":"00000000004ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e","right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"},
{"hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"}
]`,
			wantCode: http.StatusOK,
			wantBody: ` [
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","status":"OK","message":"created"},
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","status":"OK","message":"already exists"},
{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","status":"error","error":"node hash does not match to hash of children"},
{"hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e","status":"OK","message":"created"}]`,
		},
		{
			title:    "Get MiddleNode",
			req:      "/node/2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
			wantCode: http.StatusOK,
			wantBody: `{
"status":"OK",
"node":{
  "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
  "left":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
  "right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"}}`,
		},
		{
			title:    "Get Leaf",
			req:      "/node/658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusOK,
			wantBody: `{
"status":"OK",
"node":{"hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"}}`,
		},
		{
			title:    "Missing Node",
			req:      "/node/00000000004ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusNotFound,
			wantBody: `{"status":"not found"}`,
		},
		{
			title:  "rewrite leaf node with middle node",
			req:    "/node",
			method: http.MethodPost,
			body: `[
  {"hash":"390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410"},
  {
    "hash":"390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410",
    "left":"28e5cdd29d9ad96cc214c654ca8e2f4fa5576bc132e172519804a58ee4bb4d18",
    "right":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325"
  }
]`,
			wantCode: http.StatusOK,
			wantBody: `[
{"hash":"390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410","status":"OK","message":"created"},
{"hash":"390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410","status":"OK","message":"leaf node was found and upgraded to middle node"}
]`,
		},
		{
			title:    "Get middle node after upgrade from leaf",
			req:      "/node/390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410",
			wantCode: http.StatusOK,
			wantBody: `{
"status":"OK",
"node":{
  "hash":"390a8a2ba18c54cca77f2c956b9293da20237b88de980b0c99ead0447e10d410",
  "left":"28e5cdd29d9ad96cc214c654ca8e2f4fa5576bc132e172519804a58ee4bb4d18",
  "right":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325"
}}`,
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.title, func(t *testing.T) {

			var bodyReader io.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			}
			method := http.MethodGet
			if tc.method != "" {
				method = method
			}
			req, err := http.NewRequest(tc.method, tc.req, bodyReader)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, tc.wantCode, rr.Code, rr.Body.String())
			require.JSONEq(t, tc.wantBody, rr.Body.String(), rr.Body.String())

		})
	}
}
