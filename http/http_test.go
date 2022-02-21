package http

import (
	"context"
	stderr "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

type nodesStorageMock struct {
	nodes        map[merkletree.Hash]hashdb.Node
	byHashErrors map[merkletree.Hash]error
}

func (n *nodesStorageMock) SaveMiddleNode(ctx context.Context,
	node hashdb.MiddleNode) (bool, error) {
	panic("implement me")
}

func (n *nodesStorageMock) SaveLeaf(ctx context.Context,
	node hashdb.Leaf) (bool, error) {
	panic("implement me")
}

func (n *nodesStorageMock) ByHash(ctx context.Context,
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
			wantBody: `{"hash":"2c32381aebce52c0c5c5a1fb92e726f66d977b58a1c8a0c14bb31ef968187325","left":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e","right":"e809a4ed2cf98922910e456f1e56862bb958777f5ff0ea6799360113257f220f"}`,
		},
		{
			title:    "Get Leaf",
			req:      "/node/658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e",
			wantCode: http.StatusOK,
			wantBody: `{"hash":"658c7a65594ebb0815e1cc20f54284ccdb51bb1625f103c116ce58444145381e"}`,
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
			require.Equal(t, tc.wantBody, rr.Body.String())
		})
	}
}
