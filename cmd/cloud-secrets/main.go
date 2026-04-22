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

var (
	Version   = "0.1.0"
	BuildDate = "2026-04-22 20:46:00"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	slog.Info("[main] starting cloud-secrets", slog.String("version", Version), slog.String("build_date", BuildDate))

	slog.Info("[main] parse config")

	// parse with generics
	cfg, err := env.ParseAs[config.Config]()
	if err != nil {
		slog.Error("[main] failed to parse config", slog.Any("err", err))
		os.Exit(1)
	}

	if err = cfg.Validate(); err != nil {
		slog.Error("[main] failed to validate config", slog.Any("err", err))
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.CloudSecrets.Log.Level,
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

	metricsGroup.BuildInfo.Set(Version, BuildDate)

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
		entrypoint.HTTPServer("health-server", createHealthServer()),
	})
	if err != nil {
		slog.Error("[main] failed to run entrypoints", slog.Any("err", err))
		os.Exit(1)
	}
}

const metricsReadHeaderTimeout = 10 * time.Second

func createHealthServer() *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:              ":8000",
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTimeout,
	}
}
