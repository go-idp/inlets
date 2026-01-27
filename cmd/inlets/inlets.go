package main

import (
	"os"

	"github.com/go-idp/inlets"
	"github.com/go-zoox/logger"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:    "inlets",
		Usage:   "Cloud Native Tunnel Server/Client",
		Version: inlets.Version,
		Commands: []*cli.Command{
			Client(),
			Server(),
			Forward(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		logger.Fatal("%s", err.Error())
	}
}
