package main

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	MySecret string `env:"MY_SECRET,file,required,notEmpty,unset"`
}

func main() {
	slog.Info("[main] parse config")

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		panic(err)
	}

	slog.Info("[main] running http server")

	http.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(writer, "MySecret=%q", cfg.MySecret)
	})

	err = http.ListenAndServe(":8080", nil) //nolint:gosec // it example app
	if err != nil {
		slog.Error("failed to listen", slog.Any("err", err))
	}
}
