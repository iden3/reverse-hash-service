package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

const sugaredLogFormat = `[%s] "%s %s %s" from %s - %s %dB in %s`

func Logger(logger *zap.SugaredLogger,
	name string) func(next http.Handler) http.Handler {
	logger = zap.New(logger.Desugar().Core(),
		zap.AddCallerSkip(1)).Sugar().Named(name)
	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			t1 := time.Now()
			defer func() {
				logger.Infof(sugaredLogFormat,
					middleware.GetReqID(r.Context()), // RequestID (if set)
					r.Method,                         // Method
					r.URL.Path,                       // Path
					r.Proto,                          // Protocol
					r.RemoteAddr,                     // RemoteAddr
					statusLabel(ww.Status()),         // "200 OK"
					ww.BytesWritten(),                // Bytes Written
					time.Since(t1),                   // Elapsed
				)
			}()
			next.ServeHTTP(ww, r)
		}
		return http.HandlerFunc(fn)
	}
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return fmt.Sprintf("%d OK", status)
	case status >= 300 && status < 400:
		return fmt.Sprintf("%d Redirect", status)
	case status >= 400 && status < 500:
		return fmt.Sprintf("%d Client Error", status)
	case status >= 500:
		return fmt.Sprintf("%d Server Error", status)
	default:
		return fmt.Sprintf("%d Unknown", status)
	}
}
