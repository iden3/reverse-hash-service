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
		_, _ = w.Write([]byte("OK"))
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
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(jsonErr(err.Error())))
			return
		}

		node, err := storage.ByHash(r.Context(), nodeHash)
		if stderr.Is(err, hashdb.ErrDoesNotExists) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":"` + statusNotFound + `"}`))
			return
		} else if err != nil {
			log.Printf("%+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(jsonErr(err.Error())))
			return
		}

		jsonResp(w, nodeResponse{node, statusOK})
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
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(jsonErr(err.Error())))
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
			rs = append(rs,
				respItem{Hash: hexHash(hash), Status: "error", Error: errMsg})
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
				respItem{Hash: hexHash(hash), Status: "OK", Message: msg})
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

		jsonResp(w, rs)
	}
}

func jsonErr(e string) string {
	res, err := json.Marshal(map[string]interface{}{
		"status": "error",
		"error":  e,
	})
	if err != nil {
		log.Printf("[assertion] why?")
		return e
	}
	return string(res)
}

func jsonResp(w http.ResponseWriter, in interface{}) {
	data, err := json.Marshal(in)
	if err != nil {
		log.Printf("%+v", errors.WithStack(err))
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(jsonErr(err.Error())))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
