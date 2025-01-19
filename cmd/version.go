package main

import (
	"fmt"

	"github.com/flashbots/amp-alerts-sink/config"
	"github.com/urfave/cli/v2"
)

func CommandVersion(cfg *config.Config) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Prints application version",

		Action: func(clictx *cli.Context) error {
			fmt.Println(clictx.App.Version)
			return nil
		},
	}
}
