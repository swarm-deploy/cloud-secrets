package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli"
)

func main() {
	app, err := cli.NewApplication()
	if err != nil {
		slog.Error("failed to create application", slog.Any("err", err))
		os.Exit(1)
	}

	app.Run(context.Background())
}
