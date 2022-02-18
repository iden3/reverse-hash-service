package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v4"
	"github.com/spf13/viper"
)

// config settings
const (
	cfgDb = "db"
)

func setupConfig() {

}

func main() {
	v := viper.NewWithOptions()
	v.SetEnvPrefix("rhs")
	v.AutomaticEnv()
	v.AddConfigPath(".")
	v.SetDefault(cfgDb, "database=rhs")
	err := v.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error
		} else {
			panic(err)
		}
	}
	fmt.Println(v.AllSettings())

	conn, err := pgx.Connect(context.Background(), v.GetString(cfgDb))
	if err != nil {
		panic(err)
	}

	fmt.Println("OK")

	err = conn.Close(context.Background())
	if err != nil {
		panic(err)
	}
}
