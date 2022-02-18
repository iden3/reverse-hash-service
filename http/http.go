package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func New(listenAddr string) Srv {
	r := chi.NewRouter()
	// Liveness probe
	r.HandleFunc("/ping", getPingHandler())
	var s srv
	s.s = &http.Server{Addr: listenAddr, Handler: r}
	return &s
}

func getPingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}
}
