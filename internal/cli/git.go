package cli

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/spf13/cobra"
)

func newGitCommand() *cobra.Command {
	command := &cobra.Command{Use: "git", Short: "Prepare and create local Git review commits"}
	command.AddCommand(newGitStatusCommand(), newGitPrepareCommand(), newGitCreateBranchCommand(), newGitCommitCommand())
	return command
}

func newGitStatusCommand() *cobra.Command {
	var workspace string
	command := &cobra.Command{Use: "status", Short: "Show Git review status", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		result, err := service.GitStatus(context.Background())
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	return command
}

func newGitPrepareCommand() *cobra.Command {
	var workspace, environment, slug, planID string
	command := &cobra.Command{Use: "prepare", Short: "Generate a local Git review artifact", RunE: func(command *cobra.Command, _ []string) error {
		if environment == "" {
			return errors.New("--environment is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		result, err := service.GitPrepare(context.Background(), app.GitPrepareInput{EnvironmentID: environment, Slug: slug, PlanID: planID})
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environment, "environment", "", "environment ID")
	command.Flags().StringVar(&slug, "slug", "", "branch topic slug")
	command.Flags().StringVar(&planID, "plan-id", "", "optional plan ID")
	return command
}

func newGitCreateBranchCommand() *cobra.Command {
	var workspace, branch, key string
	command := &cobra.Command{Use: "create-branch", Short: "Create the explicitly requested local Git branch", RunE: func(command *cobra.Command, _ []string) error {
		if branch == "" {
			return errors.New("--branch is required")
		}
		if key == "" {
			return errors.New("--idempotency-key is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		result, err := service.GitCreateBranch(context.Background(), branch, key)
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&branch, "branch", "", "new branch name")
	command.Flags().StringVar(&key, "idempotency-key", "", "required idempotency key")
	return command
}

func newGitCommitCommand() *cobra.Command {
	var workspace, message, key string
	var files []string
	command := &cobra.Command{Use: "commit", Short: "Commit explicitly listed managed files", RunE: func(command *cobra.Command, _ []string) error {
		if message == "" {
			return errors.New("--message is required")
		}
		if key == "" {
			return errors.New("--idempotency-key is required")
		}
		if len(files) == 0 {
			return errors.New("--file is required")
		}
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		result, err := service.GitCommit(context.Background(), files, message, key)
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(result)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&message, "message", "", "English Conventional Commit message")
	command.Flags().StringSliceVar(&files, "file", nil, "managed file to commit; repeat for multiple files")
	command.Flags().StringVar(&key, "idempotency-key", "", "required idempotency key")
	return command
}
