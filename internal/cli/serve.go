package cli

import (
	"fmt"
	"net"
	"net/http"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/server"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var workspace string
	var address string
	command := &cobra.Command{
		Use:   "serve",
		Short: "Serve the local Conflow web UI",
		RunE: func(command *cobra.Command, args []string) error {
			if err := validateListenAddress(address); err != nil {
				return err
			}
			service, err := app.Open(workspace)
			if err != nil {
				return err
			}
			listener, err := net.Listen("tcp", address)
			if err != nil {
				return err
			}
			defer listener.Close()

			fmt.Fprintf(command.OutOrStdout(), "Conflow is listening on http://%s\n", listener.Addr())
			return http.Serve(listener, server.New(service))
		},
	}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&address, "address", "127.0.0.1:9010", "loopback listen address")
	return command
}

func validateListenAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address must use a loopback host")
	}
	return nil
}
