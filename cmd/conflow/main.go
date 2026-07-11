package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/ConteMan/conflow/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.New(version).Execute(); err != nil {
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
