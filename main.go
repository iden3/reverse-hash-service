package main

import (
	"context"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/iden3/reverse-hash-service/http"
	"github.com/iden3/reverse-hash-service/log"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// config settings
const (
	cfgDb         = "db"
	cfgListenAddr = "listen-addr"
)

func setupConfig() *viper.Viper {
	v := viper.NewWithOptions()
	v.SetEnvPrefix("rhs")
	v.AutomaticEnv()
	v.AddConfigPath(".")
	v.SetDefault(cfgDb, "database=rhs")
	v.SetDefault(cfgListenAddr, ":8080")
	err := v.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error
		} else {
			panic(err)
		}
	}
	return v
}

func main() {
	v := setupConfig()

	if err := log.Setup(); err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
	// Syncing of console causes error. Ignore any errors on Sync.
	defer func() { _ = log.Sync() }()

	conn, err := pgxpool.Connect(context.Background(), v.GetString(cfgDb))
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	storage := hashdb.New(conn)

	httpSrv := http.New(v.GetString(cfgListenAddr), storage)
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		closeWithErrLog(httpSrv, 10*time.Second)
	}()

	log.Infof("Start listening on %v", v.GetString(cfgListenAddr))
	err = httpSrv.Run()
	if err != nil {
		log.Errorw(err.Error(), zap.Error(err))
	}

	wg.Wait()
	log.Infof("Bye")
}

type ctxCloser interface {
	Close(context.Context) error
}

func closeWithErrLog(c ctxCloser, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := c.Close(ctx)
	if err != nil {
		log.Errorf("%+v", err)
	}
}
