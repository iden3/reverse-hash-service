package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
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

func New(listenAddr string, storage nodesSubmitter) Srv {
	r := chi.NewRouter()
	// Liveness probe
	r.HandleFunc("/ping", getPingHandler())
	r.Get("/node", getNodeHandler())
	r.Post("/node", getNodeSubmitHandler(storage))
	var s srv
	s.s = &http.Server{Addr: listenAddr, Handler: r}
	return &s
}

func getPingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}
}

func getNodeHandler() http.HandlerFunc {
	return nil
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

		_, _ = w.Write([]byte(`{"status": "OK"}`))
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
