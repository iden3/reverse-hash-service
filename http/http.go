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
			_, _ = w.Write([]byte(`{"status":"not found"}`))
			return
		} else if err != nil {
			log.Printf("%+v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(jsonErr(err.Error())))
			return
		}

		jsonResp(w, nodeResponse{node})
	}
}

type nodesSubmitter interface {
	SaveMiddleNode(ctx context.Context, node hashdb.MiddleNode) (bool, error)
	SaveLeaf(ctx context.Context, node hashdb.Leaf) (bool, error)
}

func getNodeSubmitHandler(storage nodesSubmitter) http.HandlerFunc {
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
		for _, n := range req {
			if n.hash == merkletree.HashZero {

				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(jsonErr("hash cannot be zero")))
				return

			} else if n.left == merkletree.HashZero &&
				n.right == merkletree.HashZero {

				_, err := storage.SaveLeaf(ctx, hashdb.Leaf(n.hash))
				if err != nil {
					log.Printf("%+v", err)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(jsonErr(err.Error())))
					return
				}

			} else {

				_, err := storage.SaveMiddleNode(ctx, hashdb.MiddleNode{
					Hash:  n.hash,
					Left:  n.left,
					Right: n.right,
				})
				if err != nil {
					log.Printf("%+v", err)
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(jsonErr(err.Error())))
					return
				}

			}
		}

		_, _ = w.Write([]byte(`{"status":"OK"}`))
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
