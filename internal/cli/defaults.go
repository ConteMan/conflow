package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newDefaultsCommand() *cobra.Command {
	var workspace, environmentID, format, output string
	command := &cobra.Command{Use: "defaults", Short: "Export client defaults from a protected remote snapshot"}
	download := &cobra.Command{Use: "download", Short: "Download XML, JSON, or plist defaults", RunE: func(command *cobra.Command, _ []string) error {
		if format != "xml" && format != "json" && format != "plist" {
			return fmt.Errorf("--format must be xml, json, or plist")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		content, _, _, err := service.Defaults(context.Background(), environmentID, format)
		if err != nil {
			return err
		}
		if output == "" {
			_, err = command.OutOrStdout().Write(content)
			return err
		}
		return os.WriteFile(output, content, 0o600)
	}}
	download.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	download.Flags().StringVar(&environmentID, "environment", "", "environment ID")
	download.Flags().StringVar(&format, "format", "", "output format: xml, json, or plist")
	download.Flags().StringVar(&output, "output", "", "output file; defaults to stdout")
	_ = download.MarkFlagRequired("environment")
	_ = download.MarkFlagRequired("format")
	command.AddCommand(download)
	return command
}
