package cli

import (
	"context"

	go_console "github.com/DrSmithFr/go-console"
)

type Application struct {
	commands []Command
}

func NewApplication() *Application {
	app := &Application{}

	app.createCommands()

	return app
}

func (app *Application) Run(ctx context.Context) {
	rootCmd := go_console.Command{
		Description: "cloud-secrets CLI",
		Scripts:     app.createScripts(ctx),
		BuildInfo: &go_console.BuildInfo{
			Name:    "cloud-secrets",
			Version: "0.3.1",
		},
	}

	app.createCommands()

	rootCmd.Run()
}

func (app *Application) createScripts(ctx context.Context) []*go_console.Script {
	scripts := make([]*go_console.Script, len(app.commands))

	for i, command := range app.commands {
		definition := command.Definition()

		script := &go_console.Script{
			Name:        definition.Name,
			Description: definition.Description,
			Runner: func(cmd *go_console.Script) go_console.ExitCode {
				err := command.Run(ctx, cmd)
				if err != nil {
					cmd.PrintError(err.Error())
					return go_console.ExitError
				}
				return go_console.ExitSuccess
			},
		}

		scripts[i] = script
	}

	return scripts
}

func (app *Application) createCommands() {
	app.commands = []Command{
		&SyncCommand{},
	}
}
