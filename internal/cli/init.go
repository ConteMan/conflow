package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	var directory, projectID, projectName, environmentID, environmentName, environmentKind, providerProjectID string
	var nonInteractive bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Create a Conflow project through the onboarding wizard",
		RunE: func(command *cobra.Command, _ []string) error {
			input := initInput{
				ProjectID: projectID, ProjectName: projectName, EnvironmentID: environmentID,
				EnvironmentName: environmentName, EnvironmentKind: environmentKind, ProviderProjectID: providerProjectID,
			}
			if nonInteractive {
				if err := input.requireNonInteractive(); err != nil {
					return err
				}
			} else if input.needsWizard() {
				var err error
				input, err = promptInit(command.InOrStdin(), command.OutOrStdout(), input)
				if err != nil {
					// Preserve the original zero-argument local quick start for
					// scripts that predate the wizard. Explicit non-interactive
					// mode remains strict and returns exit 64 for missing flags.
					if errors.Is(err, io.EOF) && projectID == "" && projectName == "" {
						path, createErr := project.CreateExample(directory)
						if createErr != nil {
							return createErr
						}
						fmt.Fprintln(command.OutOrStdout(), path)
						return nil
					}
					return usageError("usage_error", "无法读取初始化向导输入；非交互执行请使用 --non-interactive 并提供必填 flag")
				}
			}
			path, err := app.Initialize(directory, input.manifest())
			if err != nil {
				return err
			}
			fmt.Fprintln(command.OutOrStdout(), path)
			return nil
		},
	}
	command.Flags().StringVar(&directory, "dir", ".", "project directory")
	command.Flags().StringVar(&projectID, "project-id", "", "stable project ID")
	command.Flags().StringVar(&projectName, "project-name", "", "project display name")
	command.Flags().StringVar(&environmentID, "environment-id", "development", "initial environment ID")
	command.Flags().StringVar(&environmentName, "environment-name", "Development", "initial environment display name")
	command.Flags().StringVar(&environmentKind, "environment-kind", "development", "initial environment kind")
	command.Flags().StringVar(&providerProjectID, "provider-project-id", "", "Firebase project ID; can be completed later in environment management")
	command.Flags().BoolVar(&nonInteractive, "non-interactive", false, "require flags instead of prompting")
	return command
}

type initInput struct {
	ProjectID, ProjectName, EnvironmentID, EnvironmentName, EnvironmentKind, ProviderProjectID string
}

func (input initInput) needsWizard() bool {
	return strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.ProjectName) == ""
}

func (input initInput) requireNonInteractive() error {
	for flag, value := range map[string]string{
		"project-id": input.ProjectID, "project-name": input.ProjectName, "environment-id": input.EnvironmentID,
		"environment-name": input.EnvironmentName, "environment-kind": input.EnvironmentKind,
	} {
		if strings.TrimSpace(value) == "" {
			return usageError("usage_error", "--"+flag+" is required with --non-interactive")
		}
	}
	return nil
}

func (input initInput) manifest() project.Manifest {
	return project.Manifest{
		Version: 1,
		Project: project.Project{ID: strings.TrimSpace(input.ProjectID), Name: strings.TrimSpace(input.ProjectName), ReleaseConfirmationPolicy: project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "environment_id"}},
		Pack:    project.PackReference{ID: "mobile-ad-monetization/v1"},
		Source:  project.Source{Type: "managed-file"},
		Environments: []project.Environment{{
			ID: strings.TrimSpace(input.EnvironmentID), Name: strings.TrimSpace(input.EnvironmentName), Kind: strings.TrimSpace(input.EnvironmentKind),
			Provider: project.Provider{Type: "firebase-remote-config", ProjectID: strings.TrimSpace(input.ProviderProjectID)},
			Publish:  project.Publish{RequiresConfirmation: strings.TrimSpace(input.EnvironmentKind) == "production"},
		}},
	}
}

func promptInit(reader io.Reader, writer io.Writer, input initInput) (initInput, error) {
	scanner := bufio.NewScanner(reader)
	for _, question := range []struct {
		label, defaultValue string
		target              *string
	}{
		{"项目 ID", input.ProjectID, &input.ProjectID},
		{"项目名称", input.ProjectName, &input.ProjectName},
		{"初始环境 ID", input.EnvironmentID, &input.EnvironmentID},
		{"初始环境名称", input.EnvironmentName, &input.EnvironmentName},
		{"初始环境类型", input.EnvironmentKind, &input.EnvironmentKind},
		{"Firebase 项目 ID（可稍后填写）", input.ProviderProjectID, &input.ProviderProjectID},
	} {
		if question.defaultValue == "" {
			fmt.Fprintf(writer, "%s: ", question.label)
		} else {
			fmt.Fprintf(writer, "%s [%s]: ", question.label, question.defaultValue)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return initInput{}, err
			}
			return initInput{}, io.EOF
		}
		value := strings.TrimSpace(scanner.Text())
		if value != "" {
			*question.target = value
		}
	}
	return input, input.requireNonInteractive()
}
