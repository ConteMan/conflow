package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/operation"
	"github.com/ConteMan/conflow/internal/provider"
	"github.com/spf13/cobra"
)

func newProviderCommand() *cobra.Command {
	command := &cobra.Command{Use: "provider", Short: "Inspect provider connectivity"}
	var workspace, environment string
	status := &cobra.Command{Use: "status", Short: "Show provider status", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		info, err := service.ProviderStatus(context.Background(), environment)
		if err != nil {
			return err
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(info)
	}}
	status.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	status.Flags().StringVar(&environment, "environment", "", "environment ID")
	_ = status.MarkFlagRequired("environment")
	var connectWorkspace, connectEnvironment, credentialsPath string
	connect := &cobra.Command{Use: "connect", Short: "Validate and save a local provider credential reference", RunE: func(command *cobra.Command, _ []string) error {
		if credentialsPath == "" {
			return usageError("usage_error", "--path is required")
		}
		service, err := app.Open(connectWorkspace)
		if err != nil {
			return err
		}
		op, err := service.StartProviderConnect(context.Background(), connectEnvironment, credentialsPath)
		if err != nil {
			return providerConnectError(err)
		}
		if op.Status != "succeeded" {
			if op.Failure != nil {
				return &providerOperationError{Code: op.Failure.Code}
			}
			return &providerOperationError{Code: "unknown"}
		}
		_, environmentInfo, err := service.GetEnvironment(context.Background(), connectEnvironment)
		if err != nil {
			return err
		}
		result := providerConnectResult{
			EnvironmentID:          connectEnvironment,
			FirebaseProjectID:      environmentInfo.Provider.ProjectID,
			CredentialsPathDisplay: provider.CredentialPathDisplay(credentialsPath),
			NextStep:               "conflow pull --environment " + connectEnvironment + " 拉取线上配置",
		}
		if jsonMode(command) {
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		}
		fmt.Fprintf(command.OutOrStdout(), "已连接 %s 环境的 Firebase（%s）\n", result.EnvironmentID, result.FirebaseProjectID)
		fmt.Fprintf(command.OutOrStdout(), "凭据引用：%s（只保存路径，不复制文件内容）\n", result.CredentialsPathDisplay)
		fmt.Fprintf(command.OutOrStdout(), "下一步：%s\n", result.NextStep)
		return nil
	}}
	connect.Flags().StringVar(&connectWorkspace, "workspace", ".", "project workspace")
	connect.Flags().StringVar(&connectEnvironment, "environment", "", "environment ID")
	connect.Flags().StringVar(&credentialsPath, "path", "", "local service account JSON path")
	_ = connect.MarkFlagRequired("environment")
	command.AddCommand(status, connect)
	return command
}

type providerConnectResult struct {
	EnvironmentID          string `json:"environment_id"`
	FirebaseProjectID      string `json:"firebase_project_id"`
	CredentialsPathDisplay string `json:"credentials_path_display"`
	NextStep               string `json:"next_step"`
}

func providerConnectError(err error) error {
	if errors.Is(err, app.ErrProviderProjectIDMissing) {
		return &ExitError{Code: ExitValidation, ErrorCode: "provider_project_id_required", Message: "错误：先在环境管理中填写 Firebase 项目 ID"}
	}
	if errors.Is(err, provider.ErrCredentialFileMissing) || errors.Is(err, provider.ErrCredentialFileUnreadable) || errors.Is(err, provider.ErrCredentialJSONInvalid) || errors.Is(err, provider.ErrCredentialServiceAccount) || errors.Is(err, provider.ErrCredentialFieldsMissing) {
		return &ExitError{Code: ExitValidation, ErrorCode: "credential_validation_failed", Message: "错误：" + provider.CredentialErrorMessage(err)}
	}
	return err
}

func newPullCommand() *cobra.Command {
	var workspace, environment string
	command := &cobra.Command{Use: "pull", Short: "Pull and protect the current remote template", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		op, err := service.StartPull(context.Background(), environment)
		if err != nil {
			return err
		}
		final, err := waitOperation(service, op.OperationID)
		if err != nil {
			return err
		}
		if final.Status != "succeeded" {
			if final.Failure != nil {
				return &providerOperationError{Code: final.Failure.Code}
			}
			return &providerOperationError{Code: "unknown"}
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(final)
	}}
	command.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	command.Flags().StringVar(&environment, "environment", "", "environment ID")
	_ = command.MarkFlagRequired("environment")
	return command
}

func newRemoteCommand() *cobra.Command {
	remote := &cobra.Command{Use: "remote", Short: "Remote provider operations"}
	var workspace, environment, planID string
	validate := &cobra.Command{Use: "validate", Short: "Validate a ready plan without publishing", RunE: func(command *cobra.Command, _ []string) error {
		service, err := app.Open(workspace)
		if err != nil {
			return err
		}
		op, err := service.StartRemoteValidate(context.Background(), environment, planID)
		if err != nil {
			return err
		}
		final, err := waitOperation(service, op.OperationID)
		if err != nil {
			return err
		}
		if final.Status != "succeeded" {
			if final.Failure != nil {
				return &providerOperationError{Code: final.Failure.Code}
			}
			return &providerOperationError{Code: "unknown"}
		}
		return json.NewEncoder(command.OutOrStdout()).Encode(final)
	}}
	validate.Flags().StringVar(&workspace, "workspace", ".", "project workspace")
	validate.Flags().StringVar(&environment, "environment", "", "environment ID")
	validate.Flags().StringVar(&planID, "plan", "", "ready plan ID")
	_ = validate.MarkFlagRequired("environment")
	_ = validate.MarkFlagRequired("plan")
	remote.AddCommand(validate)
	return remote
}

func waitOperation(service *app.Service, id string) (operation.Operation, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		op, err := service.Operation(context.Background(), id)
		if err != nil {
			return operation.Operation{}, err
		}
		if op.Status == "succeeded" || op.Status == "failed" || op.Status == "cancelled" {
			return op, nil
		}
		if time.Now().After(deadline) {
			return operation.Operation{}, fmt.Errorf("operation timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
