package main

import (
	"github.com/go-idp/inlets/internal/forward"
	"github.com/urfave/cli/v2"
)

func Forward() *cli.Command {
	return &cli.Command{
		Name:  "forward",
		Usage: "forward tcp proxy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "source",
				Usage:    "source address",
				Aliases:  []string{"s"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "target",
				Usage:    "target address",
				Aliases:  []string{"t"},
				Required: true,
			},
		},
		Action: func(c *cli.Context) error {
			forward.New(c.String("source"), c.String("target")).Start()
			return nil
		},
	}
}
