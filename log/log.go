package log

import (
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

var logger *zap.SugaredLogger

// Setup creates new global logger. It is not thread safe.
func Setup() error {
	l, err := zap.NewDevelopment()
	if err != nil {
		return errors.WithStack(err)
	}
	logger = l.Sugar()
	return nil
}

func Sync() error {
	return logger.Sync()
}

func Infof(format string, v ...interface{}) {
	logger.Infof(format, v...)
}

func Errorf(format string, v ...interface{}) {
	logger.Errorf(format, v...)
}
