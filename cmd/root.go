package cmd

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "websudo",
		Short: "Policy-aware reverse proxy for controlled access to web services",
		Long:  "websudo is a policy-aware reverse proxy that brokers scoped access to web services without exposing real credentials.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newVersionCmd(version))
	cmd.AddCommand(newServeCmd())

	return cmd
}

// Execute invokes the command.
func Execute(version string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cmd := newRootCmd(version)
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		return fmt.Errorf("error executing root command: %w", err)
	}

	return nil
}
