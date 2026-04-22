package application

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	dock "github.com/moby/moby/client"
	"github.com/swarm-deploy/cloud-secrets/internal/config"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secrets"
)

type Application struct {
	cfg config.Config

	metrics *metrics.Group

	secretProvider contracts.Provider

	ticker *time.Ticker

	docker dock.APIClient

	synchronizer *secrets.Synchronizer

	synchronizing atomic.Bool
}

func NewApplication(ctx context.Context, cfg config.Config, metricsGroup *metrics.Group) (*Application, error) {
	app := &Application{
		cfg:     cfg,
		metrics: metricsGroup,
	}

	dockerClient, err := dock.New(dock.FromEnv, dock.WithAPIVersionFromEnv())
	if err != nil {
		return nil, fmt.Errorf("[main] failed to create docker client: %w", err)
	}

	app.docker = dockerClient

	slog.Info("[app] creating secret provider", slog.String("provider", string(cfg.CloudSecrets.Provider)))

	provider, err := providers.Create(ctx, cfg, metricsGroup.Provider)
	if err != nil {
		return nil, fmt.Errorf("create secret provider: %w", err)
	}

	app.secretProvider = provider

	app.synchronizer = secrets.NewSynchronizer(
		engine.NewClient(dockerClient, metricsGroup.Docker),
		provider,
		metricsGroup.Secrets,
	)

	return app, nil
}

const sighupBuf = 3

func (app *Application) Run(ctx context.Context) error {
	slog.InfoContext(ctx, "setup ticker", slog.String("interval", app.cfg.CloudSecrets.RefreshInterval.String()))

	app.ticker = time.NewTicker(app.cfg.CloudSecrets.RefreshInterval)

	sighupChannel := make(chan os.Signal, sighupBuf)

	signal.Notify(sighupChannel, syscall.SIGHUP)

	runSync := func(channel string) {
		if !app.synchronizing.CompareAndSwap(false, true) {
			slog.InfoContext(ctx, fmt.Sprintf("skip sync by %s", channel))
			return
		}

		defer app.synchronizing.Swap(false)

		slog.DebugContext(ctx, "run sync")

		app.metrics.Syncs.RecordRun(channel)

		result, err := app.synchronizer.Sync(ctx)
		app.metrics.Syncs.SetLastSyncAt(time.Now())
		if err != nil {
			slog.ErrorContext(ctx, "failed to sync secrets", slog.Any("err", err))
		}

		slog.Info("sync finished",
			slog.Int("secrets_created", result.Created),
			slog.Int("secrets_updated", result.Updated),
			slog.Int("secrets_skipped", result.Skipped),
		)
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "sync stopped")

			return nil
		case <-sighupChannel:
			slog.InfoContext(ctx, "received SIGHUP, run sync")

			runSync("sighup")
		case <-app.ticker.C:
			runSync("interval")
		}
	}
}

func (app *Application) Close() error {
	if app.ticker != nil {
		app.ticker.Stop()
	}
	if app.docker != nil {
		err := app.docker.Close()
		if err != nil {
			return fmt.Errorf("close docker client: %w", err)
		}
	}

	return nil
}
