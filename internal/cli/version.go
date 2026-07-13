package cli

import (
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// BuildInfo carries version metadata injected at build time via ldflags.
type BuildInfo struct {
	Version   string
	Commit    string
	BuildTime string
}

func resolveBuildInfo(info BuildInfo) BuildInfo {
	if v := strings.TrimSpace(info.Version); v == "" || v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi != nil {
			candidate := strings.TrimSpace(bi.Main.Version)
			if candidate != "" && candidate != "(devel)" {
				info.Version = candidate
			}
		}
	}
	if strings.TrimSpace(info.Version) == "" {
		info.Version = "dev"
	}
	return info
}

func newVersionCommand(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Conflow version",
		Run: func(command *cobra.Command, args []string) {
			resolved := resolveBuildInfo(info)
			if jsonMode(command) {
				type versionOutput struct {
					Version   string `json:"version"`
					Commit    string `json:"commit,omitempty"`
					BuildTime string `json:"build_time,omitempty"`
				}
				_ = json.NewEncoder(command.OutOrStdout()).Encode(versionOutput{
					Version:   resolved.Version,
					Commit:    resolved.Commit,
					BuildTime: resolved.BuildTime,
				})
				return
			}
			fmt.Fprintln(command.OutOrStdout(), resolved.Version)
		},
	}
}
