package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/artarts36/go-entrypoint"
	"github.com/caarlos0/env/v11"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/swarm-deploy/cloud-secrets/internal/application"
	"github.com/swarm-deploy/cloud-secrets/internal/config"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
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

	initCtx, cancel := context.WithTimeout(context.Background(), time.Minute)

	metricsGroup := metrics.NewGroup(metrics.CreateGroupParams{
		Namespace: "cloud_secrets",
	})
	if err = prometheus.Register(metricsGroup); err != nil {
		slog.Error("[main] failed to register metrics", slog.Any("err", err))
		os.Exit(1)
	}

	app, err := application.NewApplication(initCtx, cfg, metricsGroup)
	if err != nil {
		slog.Error("[main] failed to create application", slog.Any("err", err))
		os.Exit(1)
	}
	cancel()

	slog.Info("[main] running application")

	err = entrypoint.Run([]entrypoint.Entrypoint{
		{
			Name: "application",
			Run:  app.Run,
			Stop: func(context.Context) error {
				return app.Close()
			},
		},
		entrypoint.HTTPServer("health-server", createMetricsServer()),
	})
	if err != nil {
		slog.Error("[main] failed to run entrypoints", slog.Any("err", err))
		os.Exit(1)
	}
}

const metricsReadHeaderTimeout = 10 * time.Second

func createMetricsServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:              ":8001",
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTimeout,
	}
}
