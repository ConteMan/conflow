package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newReleaseCommand() *cobra.Command {
	var workspace, environmentID string
	command := &cobra.Command{Use: "release", Short: "Inspect local release audit records"}
	list := &cobra.Command{Use: "list", Short: "List release audit records", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		items, err := service.ReleasesPage(context.Background(), environmentID, 0, "")
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(items)
	}}
	show := &cobra.Command{Use: "show <release_id>", Args: cobra.ExactArgs(1), Short: "Show one release audit record", RunE: func(command *cobra.Command, args []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		item, found, err := service.ReleaseWithError(context.Background(), args[0])
		if err != nil {
			return err
		}
		if !found || item.EnvironmentID != environmentID {
			return fmt.Errorf("release %s not found", args[0])
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(item)
	}}
	for _, child := range []*cobra.Command{list, show} {
		child.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
		child.Flags().StringVar(&environmentID, "environment", "", "environment ID")
		_ = child.MarkFlagRequired("environment")
		command.AddCommand(child)
	}
	return command
}
