package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ConteMan/conflow/internal/cli"
)

var (
	version   = "dev"
	commit    = ""
	buildTime = ""
)

func main() {
	if err := cli.NewWithBuildInfo(cli.BuildInfo{
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
	}).Execute(); err != nil {
		var exitError *cli.ExitError
		if errors.As(err, &exitError) {
			if !exitError.JSON && exitError.Error() != "" {
				fmt.Fprintln(os.Stderr, exitError)
			}
			os.Exit(exitError.Code)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
