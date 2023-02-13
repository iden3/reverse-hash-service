package http

import (
	"context"
	"encoding/json"
	stderr "errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/iden3/reverse-hash-service/log"
	"github.com/pkg/errors"
	"go.uber.org/zap"
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
	r.Use(middleware.RequestID)
	r.Use(Logger(log.Logger, ""))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "X-CSRF-Token"},
		MaxAge:         300,
	}))
	r.HandleFunc("/ping", getPingHandler()) // Liveness probe
	r.Get("/node/{"+paramHash+"}", getNodeHandler(storage))
	r.Post("/node", getNodeSubmitHandler(storage))
	return r
}

func getPingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResp(r.Context(), w, http.StatusOK,
			map[string]interface{}{keyStatus: statusOK})
	}
}

type nodesGetter interface {
	ByHash(ctx context.Context, hash merkletree.Hash) (hashdb.Node, error)
}

func getNodeHandler(storage nodesGetter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		var nodeHash merkletree.Hash
		err := unpackHash(&nodeHash, chi.URLParam(r, paramHash))
		if err != nil {
			jsonErr(ctx, w, http.StatusBadRequest, err.Error())
			return
		}

		node, err := storage.ByHash(r.Context(), nodeHash)
		if stderr.Is(err, hashdb.ErrDoesNotExists) {
			jsonResp(ctx, w, http.StatusNotFound,
				map[string]interface{}{keyStatus: statusNotFound})
			return
		} else if err != nil {
			log.WithContext(ctx).Errorw(err.Error(), zap.Error(err))
			jsonErr(ctx, w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResp(ctx, w, http.StatusOK, nodeResponse{node, statusOK})
	}
}

type nodesSubmitter interface {
	SaveNodes(ctx context.Context, nodes []hashdb.Node) error
}

func getNodeSubmitHandler(storage nodesSubmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req nodeSubmitRequest
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&req)
		if err != nil {
			jsonErr(ctx, w, http.StatusBadRequest, err.Error())
			return
		}

		err = storage.SaveNodes(ctx, req)
		if err != nil {
			log.WithContext(ctx).Errorw(err.Error(), zap.Error(err))
			// TODO hide real error from user and show predefined errors only
			jsonErr(ctx, w, http.StatusInternalServerError, err.Error())
			return
		}

		jsonResp(ctx, w, http.StatusOK,
			map[string]interface{}{keyStatus: statusOK})
	}
}

func jsonErr(ctx context.Context, w http.ResponseWriter, httpCode int,
	e string) {

	if httpCode == 0 {
		httpCode = http.StatusInternalServerError
	}

	jsonResp(ctx, w, httpCode, map[string]interface{}{
		keyStatus: statusError,
		keyError:  e,
	})
}

func jsonResp(ctx context.Context, w http.ResponseWriter, httpCode int,
	in interface{}) {

	data, err := json.Marshal(in)
	if err != nil {
		log.WithContext(ctx).Errorw(err.Error(),
			zap.Error(errors.WithStack(err)))
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
