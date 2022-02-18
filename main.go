package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/iden3/reverse-hash-service/http"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
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

	log.Printf("Start listening on %v", v.GetString(cfgListenAddr))
	err = httpSrv.Run()
	if err != nil {
		log.Printf("%+v", err)
	}

	wg.Wait()
	log.Print("Bye")
}

type ctxCloser interface {
	Close(context.Context) error
}

func closeWithErrLog(c ctxCloser, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := c.Close(ctx)
	if err != nil {
		log.Printf("%+v", err)
	}
}
