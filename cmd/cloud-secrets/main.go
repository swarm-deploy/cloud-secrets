package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/env/v11"
	"github.com/swarm-deploy/cloud-secrets/internal/application"
	"github.com/swarm-deploy/cloud-secrets/internal/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("[main] parse config")

	// parse with generics
	cfg, err := env.ParseAs[config.Config]()
	if err != nil {
		slog.Error("[main] failed to parse config", slog.Any("err", err))
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.SwarmSecrets.Log.Level,
	})))

	slog.Info("[main] creating application")

	ctx, cancel := context.WithCancel(context.Background())

	app, err := application.NewApplication(ctx, cfg)
	if err != nil {
		slog.Error("[main] failed to create application", slog.Any("err", err))
		os.Exit(1)
	}

	slog.Info("[main] running application")

	ch := make(chan os.Signal, 1)

	signal.Notify(ch, syscall.SIGTERM)

	go func() {
		for range ch {
			cancel()
			closeErr := app.Close()
			if closeErr != nil {
				slog.Error("[main] failed to close application", slog.Any("err", closeErr))
			}
		}
	}()

	err = app.Run(ctx)
	if err != nil {
		slog.Error("[main] failed to run application", slog.Any("err", err))
		os.Exit(1)
	}
}
