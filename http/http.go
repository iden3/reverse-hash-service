package http

import (
	"context"
	"encoding/json"
	stderr "errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
)

const (
	paramHash = "hash"
)

const (
	statusOK       = "OK"
	statusError    = "error"
	statusNotFound = "not found"
)

type Srv interface {
	Run() error
	Close(context.Context) error
}

type srv struct {
	s *http.Server
}

func (s *srv) Run() error {
	err := s.s.ListenAndServe()
	switch err {
	case http.ErrServerClosed:
		return nil
	default:
		return errors.WithStack(err)
	}
}

func (s *srv) Close(ctx context.Context) error {
	return errors.WithStack(s.s.Shutdown(ctx))
}

type nodesStorage interface {
	nodesSubmitter
	nodesGetter
}

func New(listenAddr string, storage nodesStorage) Srv {
	var s srv
	s.s = &http.Server{Addr: listenAddr, Handler: setupRouter(storage)}
	return &s
}

func setupRouter(storage nodesStorage) *chi.Mux {
	r := chi.NewRouter()
	r.HandleFunc("/ping", getPingHandler()) // Liveness probe
	r.Get("/node/{"+paramHash+"}", getNodeHandler(storage))
	r.Post("/node", getNodeSubmitHandler(storage))
	return r
}

func getPingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, http.StatusOK, map[string]interface{}{keyStatus: statusOK})
	}
}

type nodesGetter interface {
	ByHash(ctx context.Context, hash merkletree.Hash) (hashdb.Node, error)
}

func getNodeHandler(storage nodesGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var nodeHash merkletree.Hash
		err := unpackHash(&nodeHash, chi.URLParam(r, paramHash))
		if err != nil {
			jsonErr(w, http.StatusBadRequest, err.Error())
			return
		}

		node, err := storage.ByHash(r.Context(), nodeHash)
		if stderr.Is(err, hashdb.ErrDoesNotExists) {
			jsonResp(w, http.StatusNotFound,
				map[string]interface{}{keyStatus: statusNotFound})
			return
		} else if err != nil {
			log.Printf("%+v", err)
			jsonErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResp(w, http.StatusOK, nodeResponse{node, statusOK})
	}
}

type nodesSubmitter interface {
	SaveMiddleNode(ctx context.Context, node hashdb.MiddleNode) error
	SaveLeaf(ctx context.Context, node hashdb.Leaf) error
}

var errZeroHash = stderr.New("node hash is zero")

func getNodeSubmitHandler(storage nodesSubmitter) http.HandlerFunc {
	type respItem struct {
		Hash    hexHash `json:"hash"`
		Status  string  `json:"status"`
		Error   string  `json:"error,omitempty"`
		Message string  `json:"message,omitempty"`
	}
	type resp []respItem
	return func(w http.ResponseWriter, r *http.Request) {
		var req nodeSubmitRequest
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&req)
		if err != nil {
			jsonErr(w, http.StatusBadRequest, err.Error())
			return
		}

		ctx := r.Context()
		var rs resp

		appendErr := func(hash merkletree.Hash, err error) {
			// err must not be nil here
			errMsg := err.Error()
			switch {
			case errors.Is(err, hashdb.ErrIncorrectHash):
				errMsg = "node hash does not match to hash of children"
			case errors.Is(err, hashdb.ErrCollision):
				errMsg = "hash collision: found another node with this hash " +
					"but different children"
			case errors.Is(err, errZeroHash):
				// pass: default message is OK
			case errors.Is(err, hashdb.ErrMiddleNodeExists):
				errMsg = "middle node with the same hash exists"
			default:
				log.Printf("%+v", err)
			}
			rs = append(rs, respItem{
				Hash:   hexHash(hash),
				Status: statusError,
				Error:  errMsg})
		}

		appendRs := func(hash merkletree.Hash, err error) {
			var msg string
			switch {
			case err == nil:
				msg = "created"
			case errors.Is(err, hashdb.ErrAlreadyExists):
				msg = "already exists"
			case errors.Is(err, hashdb.ErrLeafUpgraded):
				msg = "leaf node was found and upgraded to middle node"
			default:
				appendErr(hash, err)
				return
			}
			rs = append(rs,
				respItem{Hash: hexHash(hash), Status: statusOK, Message: msg})
		}

	LOOP:
		for _, n := range req {

			select {
			case <-ctx.Done():
				break LOOP
			default:
			}

			if n.hash == merkletree.HashZero {
				appendErr(n.hash, errZeroHash)
			} else if n.left == merkletree.HashZero &&
				n.right == merkletree.HashZero {

				err := storage.SaveLeaf(ctx, hashdb.Leaf(n.hash))
				appendRs(n.hash, err)

			} else {

				node := hashdb.MiddleNode{
					Hash:  n.hash,
					Left:  n.left,
					Right: n.right}
				appendRs(n.hash, storage.SaveMiddleNode(ctx, node))

			}
		}

		jsonResp(w, http.StatusOK, rs)
	}
}

func jsonErr(w http.ResponseWriter, httpCode int, e string) {
	if httpCode == 0 {
		httpCode = http.StatusInternalServerError
	}

	jsonResp(w, httpCode, map[string]interface{}{
		keyStatus: statusError,
		keyError:  e,
	})
}

func jsonResp(w http.ResponseWriter, httpCode int, in interface{}) {
	data, err := json.Marshal(in)
	if err != nil {
		log.Printf("%+v", errors.WithStack(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("unable to marshal response"))
		return
	}

	if httpCode == 0 {
		httpCode = http.StatusOK
	}
	w.WriteHeader(httpCode)
	_, _ = w.Write(data)
}
