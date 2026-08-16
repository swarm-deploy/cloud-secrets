package cli

import (
	"context"

	go_console "github.com/DrSmithFr/go-console"
)

type Command interface {
	Definition() CommandDefinition
	Run(ctx context.Context, script *go_console.Script) error
}

type CommandDefinition struct {
	Name        string
	Description string
}
