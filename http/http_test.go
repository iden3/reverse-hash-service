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
	go_test_pg "github.com/olomix/go-test-pg"
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

func (n *nodesStorageMock) SaveNodes(_ context.Context, _ []hashdb.Node) error {
	panic("implement me")
}

func (n *nodesStorageMock) ByHash(_ context.Context,
	hash merkletree.Hash) (hashdb.Node, error) {

	var node hashdb.Node
	err, ok := n.byHashErrors[hash]
	if ok {
		return node, err
	}

	node, ok = n.nodes[hash]
	if !ok {
		return node, errors.WithStack(hashdb.ErrDoesNotExists)
	}
	return node, nil
}

func TestGetNodeHandler(t *testing.T) {
	node1 := mkNode(t,
		"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
		[]string{
			"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f",
		})
	node2 := mkNode(t,
		"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
		[]string{
			"037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
			"0000000000000000000000000000000000000000000000000000000000000000",
			"0100000000000000000000000000000000000000000000000000000000000000",
		})
	ng := nodesStorageMock{
		nodes: map[merkletree.Hash]hashdb.Node{
			node1.Hash: node1,
			node2.Hash: node2,
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
    "children":[
      "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
      "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
    ]
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
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "children":[
      "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
      "0000000000000000000000000000000000000000000000000000000000000000",
      "0100000000000000000000000000000000000000000000000000000000000000"
    ]
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
			req, err := http.NewRequest(http.MethodGet, tc.req, http.NoBody)
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
	}{
		{
			title:  "save few nodes",
			req:    "/node",
			method: http.MethodPost,
			body: `[
  {
    "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
    "children":[
      "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
      "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
    ]
  },
  {
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "children":[
      "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
      "0000000000000000000000000000000000000000000000000000000000000000",
      "0100000000000000000000000000000000000000000000000000000000000000"
    ]
  }
]`,
			wantCode: http.StatusOK,
			wantBody: `{"status":"OK"}`,
		},
		{
			title:  "incorrect hash",
			req:    "/node",
			method: http.MethodPost,
			body: `[
  {
    "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
    "children":[
      "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
      "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
    ]
  },
  {
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381f",
    "children":[
      "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
      "0000000000000000000000000000000000000000000000000000000000000000",
      "0100000000000000000000000000000000000000000000000000000000000000"
    ]
  }
]`,
			wantCode: http.StatusBadRequest,
			wantBody: `{"error":"error parsing node #2: node hash is not correct","status":"error"}`,
		},
		{
			title:    "get middle node",
			req:      "/node/2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
			wantCode: http.StatusOK,
			wantBody: `{
"status":"OK",
"node":{
    "hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325",
    "children":[
      "658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
      "e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"
    ]
  }
}`,
		},
		{
			title:    "Get leaf",
			req:      "/node/658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusOK,
			wantBody: `{
"status":"OK",
"node":{
    "hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
    "children":[
      "037c4d7bbb0407b8000000000000000000000000000000000000000000000000",
      "0000000000000000000000000000000000000000000000000000000000000000",
      "0100000000000000000000000000000000000000000000000000000000000000"
    ]
  }
}`,
		},
		{
			title:    "Missing Node",
			req:      "/node/00000000004ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusNotFound,
			wantBody: `{"status":"not found"}`,
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
				method = tc.method
			}
			req, err := http.NewRequest(method, tc.req, bodyReader)
			require.NoError(t, err)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, tc.wantCode, rr.Code, rr.Body.String())
			require.JSONEq(t, tc.wantBody, rr.Body.String(), rr.Body.String())

		})
	}
}

func mkNode(t testing.TB, hash string, children []string) hashdb.Node {
	var childrenH = make([]merkletree.Hash, len(children))
	for i := range children {
		childrenH[i] = hashFromHex(t, children[i])
	}
	return hashdb.Node{
		Hash:     hashFromHex(t, hash),
		Children: childrenH,
	}
}
