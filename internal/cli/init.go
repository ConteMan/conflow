package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/ConteMan/conflow/internal/app"
	"github.com/ConteMan/conflow/internal/project"
	"github.com/spf13/cobra"
)

var initIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var firebaseProjectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

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
				UseInitialEnvironment: command.Flags().Changed("environment-id") || command.Flags().Changed("environment-name") || command.Flags().Changed("environment-kind") || command.Flags().Changed("provider-project-id"),
			}
			if nonInteractive {
				if err := input.requireNonInteractive(); err != nil {
					return err
				}
			} else {
				if !isInteractiveInput(command.InOrStdin()) {
					return usageError("usage_error", "检测到非交互输入；请使用 --non-interactive 并提供必填 flag。示例：conflow init --non-interactive --dir ./my-app --project-id my-app --project-name \"My App\"")
				}
				var err error
				input, err = promptInit(command.InOrStdin(), command.OutOrStdout(), input)
				if err != nil {
					return usageError("usage_error", "无法读取初始化向导输入；请检查输入后重试")
				}
			}

			path, err := app.Initialize(directory, input.manifest())
			if err != nil {
				return err
			}
			result := initResult{ProjectPath: path, NextSteps: initNextSteps(directory)}
			if jsonMode(command) {
				return json.NewEncoder(command.OutOrStdout()).Encode(result)
			}
			fmt.Fprintf(command.OutOrStdout(), "\n已创建 %s\n\n下一步：\n", displayManifestPath(directory))
			for index, step := range result.NextSteps {
				fmt.Fprintf(command.OutOrStdout(), "  %d. %s\n", index+1, step)
			}
			return nil
		},
	}
	command.Flags().StringVar(&directory, "dir", ".", "project directory")
	command.Flags().StringVar(&projectID, "project-id", "", "stable project ID")
	command.Flags().StringVar(&projectName, "project-name", "", "project display name")
	command.Flags().StringVar(&environmentID, "environment-id", "development", "initial environment ID for non-interactive mode")
	command.Flags().StringVar(&environmentName, "environment-name", "Development", "initial environment display name for non-interactive mode")
	command.Flags().StringVar(&environmentKind, "environment-kind", "development", "initial environment kind for non-interactive mode")
	command.Flags().StringVar(&providerProjectID, "provider-project-id", "", "Firebase project ID for the initial environment")
	command.Flags().BoolVar(&nonInteractive, "non-interactive", false, "require flags instead of prompting")
	return command
}

type initInput struct {
	ProjectID, ProjectName, EnvironmentID, EnvironmentName, EnvironmentKind, ProviderProjectID string
	UseInitialEnvironment                                                                      bool
	Environments                                                                               []project.Environment
}

type initResult struct {
	ProjectPath string   `json:"project_path"`
	NextSteps   []string `json:"next_steps"`
}

func (input initInput) requireNonInteractive() error {
	if strings.TrimSpace(input.ProjectName) == "" {
		return usageError("usage_error", "--project-name is required with --non-interactive")
	}
	if err := validateProjectID(input.ProjectID); err != nil {
		return usageError("usage_error", "--project-id "+err.Error())
	}
	if err := validateEnvironment(input.EnvironmentID, input.EnvironmentName, input.EnvironmentKind, input.ProviderProjectID); err != nil {
		return usageError("usage_error", err.Error())
	}
	return nil
}

func (input initInput) manifest() project.Manifest {
	environments := input.Environments
	if len(environments) == 0 {
		initial := project.Environment{
			ID: strings.TrimSpace(input.EnvironmentID), Name: strings.TrimSpace(input.EnvironmentName), Kind: strings.TrimSpace(input.EnvironmentKind),
			Provider: project.Provider{Type: "firebase-remote-config", ProjectID: strings.TrimSpace(input.ProviderProjectID)},
			Publish:  project.Publish{RequiresConfirmation: strings.TrimSpace(input.EnvironmentKind) == "production"},
		}
		if !input.UseInitialEnvironment && initial.ID == "development" && initial.Name == "Development" && initial.Kind == "development" {
			environments = defaultInitEnvironments(initial.Provider.ProjectID)
		} else {
			environments = []project.Environment{initial}
		}
	}
	return project.Manifest{
		Version:      1,
		Project:      project.Project{ID: strings.TrimSpace(input.ProjectID), Name: strings.TrimSpace(input.ProjectName), ReleaseConfirmationPolicy: project.ReleaseConfirmationPolicy{ProductionLowRiskMode: "environment_id"}},
		Pack:         project.PackReference{ID: "mobile-ad-monetization/v1"},
		Source:       project.Source{Type: "managed-file"},
		Environments: environments,
	}
}

func defaultInitEnvironments(developmentProjectID string) []project.Environment {
	return []project.Environment{
		{ID: "development", Name: "Development", Kind: "development", Provider: project.Provider{Type: "firebase-remote-config", ProjectID: strings.TrimSpace(developmentProjectID)}},
		{ID: "production", Name: "Production", Kind: "production", Provider: project.Provider{Type: "firebase-remote-config"}, Publish: project.Publish{RequiresConfirmation: true}},
	}
}

func promptInit(reader io.Reader, writer io.Writer, input initInput) (initInput, error) {
	scanner := bufio.NewScanner(reader)
	fmt.Fprintln(writer, "欢迎使用 Conflow。回答几个问题来创建你的项目（回车使用默认值）。")
	fmt.Fprintln(writer)

	name, err := promptRequired(scanner, writer, "项目名称（显示用，如“记账应用”）", input.ProjectName, func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("项目名称不能为空")
		}
		return nil
	})
	if err != nil {
		return initInput{}, err
	}
	input.ProjectName = name

	idDefault := strings.TrimSpace(input.ProjectID)
	if idDefault == "" {
		idDefault = deriveProjectID(name)
	}
	id, err := promptRequired(scanner, writer, "项目 ID（小写字母/数字/连字符，字母开头）", idDefault, validateProjectID)
	if err != nil {
		return initInput{}, err
	}
	input.ProjectID = id

	adjust, err := promptValue(scanner, writer, "默认创建 development 和 production 两个环境，是否调整？[N/y]", "")
	if err != nil {
		return initInput{}, err
	}
	environments := defaultInitEnvironments("")
	if strings.EqualFold(adjust, "y") || strings.EqualFold(adjust, "yes") {
		environments, err = promptEnvironments(scanner, writer, environments)
		if err != nil {
			return initInput{}, err
		}
	} else {
		for index := range environments {
			label := environments[index].ID + " 的 Firebase 项目 ID（可留空"
			if index == 0 {
				label += "，稍后在界面“环境管理”中填写"
			}
			label += "）"
			value, promptErr := promptOptional(scanner, writer, label, environments[index].Provider.ProjectID, validateFirebaseProjectID)
			if promptErr != nil {
				return initInput{}, promptErr
			}
			environments[index].Provider.ProjectID = value
		}
	}
	input.Environments = environments
	return input, nil
}

func promptEnvironments(scanner *bufio.Scanner, writer io.Writer, environments []project.Environment) ([]project.Environment, error) {
	for {
		fmt.Fprintln(writer, "环境调整：输入 a 添加、d 删除，直接回车完成。")
		for _, environment := range environments {
			fmt.Fprintf(writer, "  - %s（%s，%s）\n", environment.ID, environment.Name, environment.Kind)
		}
		action, err := promptValue(scanner, writer, "操作 [a/d]", "")
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(action) {
		case "":
			return environments, nil
		case "a":
			environment, addErr := promptEnvironment(scanner, writer)
			if addErr != nil {
				return nil, addErr
			}
			if hasEnvironment(environments, environment.ID) {
				fmt.Fprintln(writer, "错误：环境 ID 已存在，请重新输入。")
				continue
			}
			environments = append(environments, environment)
		case "d":
			if len(environments) == 1 {
				fmt.Fprintln(writer, "错误：至少保留一个环境。")
				continue
			}
			id, deleteErr := promptValue(scanner, writer, "要删除的环境 ID", "")
			if deleteErr != nil {
				return nil, deleteErr
			}
			if !hasEnvironment(environments, id) {
				fmt.Fprintln(writer, "错误：环境 ID 不存在，请重新输入。")
				continue
			}
			environments = removeEnvironment(environments, id)
		default:
			fmt.Fprintln(writer, "错误：请输入 a、d 或直接回车。")
		}
	}
}

func promptEnvironment(scanner *bufio.Scanner, writer io.Writer) (project.Environment, error) {
	id, err := promptRequired(scanner, writer, "环境 ID（小写字母/数字/连字符，字母开头）", "", validateProjectID)
	if err != nil {
		return project.Environment{}, err
	}
	name, err := promptRequired(scanner, writer, "环境名称", "", func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("环境名称不能为空")
		}
		return nil
	})
	if err != nil {
		return project.Environment{}, err
	}
	kind, err := promptRequired(scanner, writer, "环境类型（development/staging/production/custom）", "development", validateEnvironmentKind)
	if err != nil {
		return project.Environment{}, err
	}
	projectID, err := promptOptional(scanner, writer, "Firebase 项目 ID（可留空）", "", validateFirebaseProjectID)
	if err != nil {
		return project.Environment{}, err
	}
	return project.Environment{ID: id, Name: name, Kind: kind, Provider: project.Provider{Type: "firebase-remote-config", ProjectID: projectID}, Publish: project.Publish{RequiresConfirmation: kind == "production"}}, nil
}

func promptRequired(scanner *bufio.Scanner, writer io.Writer, label, defaultValue string, validate func(string) error) (string, error) {
	for {
		value, err := promptValue(scanner, writer, label, defaultValue)
		if err != nil {
			return "", err
		}
		if err := validate(value); err != nil {
			fmt.Fprintf(writer, "错误：%s，请重新输入。\n", err)
			continue
		}
		return value, nil
	}
}

func promptOptional(scanner *bufio.Scanner, writer io.Writer, label, defaultValue string, validate func(string) error) (string, error) {
	for {
		value, err := promptValue(scanner, writer, label, defaultValue)
		if err != nil {
			return "", err
		}
		if value == "" {
			return "", nil
		}
		if err := validate(value); err != nil {
			fmt.Fprintf(writer, "错误：%s，请重新输入。\n", err)
			continue
		}
		return value, nil
	}
}

func promptValue(scanner *bufio.Scanner, writer io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(writer, "%s: ", label)
	} else {
		fmt.Fprintf(writer, "%s [%s]: ", label, defaultValue)
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	value := strings.TrimSpace(scanner.Text())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func validateProjectID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("项目 ID 不能为空")
	}
	if !initIDPattern.MatchString(value) {
		return fmt.Errorf("项目 ID 必须以小写字母开头，只能包含小写字母、数字和连字符，长度为 2-63")
	}
	return nil
}

func validateFirebaseProjectID(value string) error {
	if !firebaseProjectIDPattern.MatchString(strings.TrimSpace(value)) {
		return fmt.Errorf("Firebase 项目 ID 必须以小写字母开头，只能包含小写字母、数字和连字符，长度为 6-30")
	}
	return nil
}

func validateEnvironmentKind(value string) error {
	switch strings.TrimSpace(value) {
	case "development", "staging", "production", "custom":
		return nil
	default:
		return fmt.Errorf("环境类型必须是 development、staging、production 或 custom")
	}
}

func validateEnvironment(id, name, kind, firebaseProjectID string) error {
	if err := validateProjectID(id); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("环境名称不能为空")
	}
	if err := validateEnvironmentKind(kind); err != nil {
		return err
	}
	if strings.TrimSpace(firebaseProjectID) != "" {
		return validateFirebaseProjectID(firebaseProjectID)
	}
	return nil
}

func deriveProjectID(name string) string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
	candidate := strings.Join(words, "-")
	if validateProjectID(candidate) != nil {
		return ""
	}
	return candidate
}

func hasEnvironment(environments []project.Environment, id string) bool {
	for _, environment := range environments {
		if environment.ID == id {
			return true
		}
	}
	return false
}

func removeEnvironment(environments []project.Environment, id string) []project.Environment {
	for index, environment := range environments {
		if environment.ID == id {
			return append(environments[:index], environments[index+1:]...)
		}
	}
	return environments
}

func displayManifestPath(directory string) string {
	directory = strings.TrimRight(directory, "/\\")
	if directory == "" || directory == "." {
		return "./.conflow/project.yaml"
	}
	return directory + "/.conflow/project.yaml"
}

func initNextSteps(directory string) []string {
	return []string{
		"conflow serve --workspace " + directory,
		"打开 http://127.0.0.1:9010，在「配置」页按引导先创建频控策略，再创建广告位",
		"准备好 Firebase 服务账号 JSON 后，在「环境管理」或用 conflow provider connect 连接",
	}
}
