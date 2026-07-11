package source

import (
	"context"
	"os/exec"
)

// CommandRunner executes a program using an argument vector. It deliberately
// has no shell-string entry point so callers cannot accidentally interpolate
// branch names, paths, or commit messages into a shell command.
type CommandRunner interface {
	Run(context.Context, string, []string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args []string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// GitExecutor centralizes every system Git invocation used by source adapters
// and the review workflow. Tests replace Runner to assert exact argument
// boundaries without starting a shell.
type GitExecutor struct {
	Runner CommandRunner
}

var DefaultGitExecutor = GitExecutor{Runner: execCommandRunner{}}

func (e GitExecutor) Output(ctx context.Context, workspace string, args ...string) ([]byte, error) {
	runner := e.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	command := make([]string, 0, len(args)+2)
	command = append(command, "-C", workspace)
	command = append(command, args...)
	return runner.Run(ctx, "git", command)
}
