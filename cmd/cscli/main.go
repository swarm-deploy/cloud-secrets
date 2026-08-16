package main

import (
	"context"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli"
)

func main() {
	app := cli.NewApplication()

	app.Run(context.Background())
}
