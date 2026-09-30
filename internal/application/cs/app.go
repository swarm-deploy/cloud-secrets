package cs

import (
	"context"
	"errors"
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
	lastSyncAt    atomic.Pointer[time.Time]
	nextSyncAt    atomic.Pointer[time.Time]
}

// ErrSyncInProgress is returned when another synchronization is already running.
var ErrSyncInProgress = errors.New("synchronization is already in progress")

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
	app.setNextSyncAt(time.Now().Add(app.cfg.CloudSecrets.RefreshInterval))

	sighupChannel := make(chan os.Signal, sighupBuf)

	signal.Notify(sighupChannel, syscall.SIGHUP)

	runSync := func(channel string) {
		_, err := app.Sync(ctx, channel)
		if errors.Is(err, ErrSyncInProgress) {
			slog.InfoContext(ctx, "skip sync because another synchronization is running", slog.String("trigger", channel))
		}
	}

	for {
		select {
		case <-ctx.Done():
			slog.DebugContext(ctx, "sync stopped")

			return nil
		case <-sighupChannel:
			slog.InfoContext(ctx, "received SIGHUP, run sync")

			runSync("sighup")
		case tickAt := <-app.ticker.C:
			app.setNextSyncAt(tickAt.Add(app.cfg.CloudSecrets.RefreshInterval))
			runSync("interval")
		}
	}
}

// Sync runs the normal synchronization flow for the given trigger.
func (app *Application) Sync(ctx context.Context, trigger string) (sync.Result, error) {
	if !app.synchronizing.CompareAndSwap(false, true) {
		return sync.Result{}, ErrSyncInProgress
	}
	defer app.synchronizing.Store(false)

	slog.DebugContext(ctx, "run sync", slog.String("trigger", trigger))
	app.metrics.Syncs.RecordRun(trigger)

	result, err := app.synchronizer.Sync(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to sync secrets", slog.String("trigger", trigger), slog.Any("err", err))
		return result, err
	}

	now := time.Now()
	app.lastSyncAt.Store(&now)
	app.metrics.Syncs.SetLastSyncAt(now)

	slog.InfoContext(ctx, "sync finished",
		slog.String("trigger", trigger),
		slog.Int("secrets_created", result.Created),
		slog.Int("secrets_removed", result.RemovedSecrets),
		slog.Int("secret_versions_removed", result.RemovedSecretVersions),
		slog.Int("secrets_updated", result.Updated),
		slog.Int("secrets_skipped", result.Skipped),
	)

	return result, nil
}

// LastSyncAt returns the last successful synchronization time known by this process.
func (app *Application) LastSyncAt() (time.Time, bool) {
	lastSyncAt := app.lastSyncAt.Load()
	if lastSyncAt == nil {
		return time.Time{}, false
	}

	return *lastSyncAt, true
}

// NextSyncAt returns the next scheduled interval synchronization time.
func (app *Application) NextSyncAt() (time.Time, bool) {
	nextSyncAt := app.nextSyncAt.Load()
	if nextSyncAt == nil {
		return time.Time{}, false
	}

	return *nextSyncAt, true
}

func (app *Application) setNextSyncAt(nextSyncAt time.Time) {
	app.nextSyncAt.Store(&nextSyncAt)
}

// ProviderDefinition returns human-readable metadata for the configured provider.
func (app *Application) ProviderDefinition() contracts.ProviderDefinition {
	return app.secretProvider.Definition()
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
