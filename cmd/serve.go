package cmd

import (
	"fmt"

	"github.com/fopina/websudo/internal/config"
	"github.com/fopina/websudo/internal/server"
	"github.com/spf13/cobra"
)

func newServeCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:          "serve",
		Short:        "Start the websudo reverse proxy",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "loaded config for %d services\n", len(cfg.Services))
			return server.New(cfg).Run(cmd.Context())
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "config/websudo.yaml", "Path to the websudo config file")

	return cmd
}
