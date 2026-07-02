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
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/sync"
)

type Application struct {
	cfg config.Config

	metrics *metrics.Group

	secretProvider contracts.Provider

	ticker *time.Ticker

	docker dock.APIClient

	synchronizer *sync.Synchronizer

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

	slog.Info("[app] creating secret provider")

	provider, err := cloudru.NewProvider(ctx, cfg.CloudRu)
	if err != nil {
		return nil, err
	}

	app.secretProvider = contracts.WithMetrics(provider, metricsGroup.Provider)

	app.synchronizer = sync.NewSynchronizer(
		engine.NewDockerClient(dockerClient, metricsGroup.Docker),
		provider,
		metricsGroup.Secrets,
		cfg.CloudSecrets.CleanupOrphanedSecrets,
		cfg.CloudSecrets.SecretNameFolderDelimiter,
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
			slog.Int("secrets_removed", result.RemovedSecrets),
			slog.Int("secret_versions_removed", result.RemovedSecretVersions),
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
