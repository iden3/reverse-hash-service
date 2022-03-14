package log

import (
	"context"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var Logger = zap.NewNop().Sugar()

const fieldRequestID = "request-id"

// Setup creates new global logger. It is not thread safe.
func Setup() error {
	dc := zap.NewDevelopmentConfig()

	// Maybe customize console encoder one day. To write pretty message on
	// local console logging
	//
	// err := zap.RegisterEncoder("sc", NewConsoleEncoder)
	// if err != nil {
	// 	return errors.WithStack(err)
	// }
	// dc.Encoding = "sc"

	l, err := dc.Build()
	if err != nil {
		return errors.WithStack(err)
	}
	Logger = l.Sugar()
	return nil
}

func Sync() error {
	return Logger.Sync()
}

func Infof(format string, v ...interface{}) {
	Logger.Infof(format, v...)
}

func Errorf(format string, v ...interface{}) {
	Logger.Errorf(format, v...)
}

func Errorw(msg string, keysAndValues ...interface{}) {
	Logger.Errorw(msg, keysAndValues...)
}

func WithContext(ctx context.Context) *zap.SugaredLogger {
	rID := middleware.GetReqID(ctx)
	if rID != "" {
		return Logger.With(fieldRequestID, rID)
	}
	return Logger
}
