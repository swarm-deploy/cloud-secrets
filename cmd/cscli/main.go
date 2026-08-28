package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli"
)

var (
	Version   = "v0.1.0"
	BuildDate = "2026-04-22 20:46:00"
)

func main() {
	app, err := cli.NewApplication(cli.BuildInfo{
		Version: Version,
		Date: 	 BuildDate,
	})
	if err != nil {
		slog.Error("[main] failed to create application", slog.Any("err", err))
		os.Exit(1)
	}

	app.Run(context.Background())
}
