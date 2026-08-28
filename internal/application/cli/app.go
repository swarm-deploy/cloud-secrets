package cli

import (
	"context"
	"fmt"
	"os"

	go_console "github.com/DrSmithFr/go-console"
	argumentpkg "github.com/DrSmithFr/go-console/input/argument"
	"github.com/DrSmithFr/go-console/input/option"
	"github.com/DrSmithFr/go-console/output"
	"github.com/DrSmithFr/go-console/question"
	dock "github.com/moby/moby/client"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
)

type Application struct {
	buildInfo BuildInfo

	docker *engine.DockerClient

	commands []framework.Command
}

type BuildInfo struct {
	Date    string
	Version string
}

func NewApplication(buildInfo BuildInfo) (*Application, error) {
	app := &Application{
		buildInfo: buildInfo,
	}

	dockerClient, err := dock.New(dock.FromEnv, dock.WithAPIVersionFromEnv())
	if err != nil {
		return nil, fmt.Errorf("[main] failed to create docker client: %w", err)
	}

	app.docker = engine.NewDockerClient(dockerClient, metrics.NopDocker{})

	app.createCommands()

	return app, nil
}

func (app *Application) Run(ctx context.Context) {
	out := output.NewCliOutput(true, nil)

	rootCmd := go_console.Command{
		Description: "cloud-secrets CLI",
		Output:      out,
		Scripts:     app.createScripts(ctx, out),
		BuildInfo: &go_console.BuildInfo{
			Name:      "cloud-secrets",
			Version:   app.buildInfo.Version,
			BuildFlag: app.buildInfo.Date,
		},
	}

	app.createCommands()

	rootCmd.Run()
}

func (app *Application) createScripts(ctx context.Context, out output.OutputInterface) []*go_console.Script {
	scripts := make([]*go_console.Script, len(app.commands))

	commandRunner := func(command framework.Command) func(script *go_console.Script) go_console.ExitCode {
		return func(script *go_console.Script) go_console.ExitCode {
			script.Input.SetInteractive(script.Input.Option("no-interaction") != option.Defined)

			err := command.Run(ctx, &framework.Execution{
				Script:   script,
				Question: question.NewHelper(os.Stdin, out),
			})
			if err != nil {
				script.PrintError(err.Error())
				return go_console.ExitError
			}

			return go_console.ExitSuccess
		}
	}

	for commandIndex, command := range app.commands {
		definition := command.Definition()

		args := make([]go_console.Argument, len(definition.Arguments))
		opts := make([]go_console.Option, len(definition.Options))

		for i, argument := range definition.Arguments {
			args[i] = go_console.Argument{
				Name:        argument.Name,
				Value:       argumentpkg.Optional,
				Description: argument.Description,
			}
		}

		for i, option := range definition.Options {
			opts[i] = go_console.Option{
				Name: option.Name,
			}
		}

		script := &go_console.Script{
			Name:        definition.Name,
			Description: definition.Description,
			Runner:      commandRunner(command),
			Arguments:   args,
			Options:     opts,
		}

		scripts[commandIndex] = script
	}

	return scripts
}

func (app *Application) createCommands() {
	app.commands = []framework.Command{
		NewSyncCommand(app.docker),
		NewListCommand(app.docker),
		NewInitCommand(app.docker),
	}
}
