package framework

import (
	"context"
)

type Command interface {
	Definition() Definition
	Run(ctx context.Context, execution *Execution) error
}

type Definition struct {
	Name        string
	Description string
	Arguments   []Argument
	Options     []Option
}
