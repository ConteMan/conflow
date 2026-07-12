package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

var (
	ErrCredentialFileMissing    = errors.New("credential file is missing")
	ErrCredentialFileUnreadable = errors.New("credential file is unreadable")
	ErrCredentialJSONInvalid    = errors.New("credential file is not valid JSON")
	ErrCredentialServiceAccount = errors.New("credential file is not a Firebase service account")
	ErrCredentialFieldsMissing  = errors.New("credential file is missing required fields")
)

// CredentialValidationError retains only local validation metadata. It never
// carries JSON content, so callers cannot accidentally echo a private key.
type CredentialValidationError struct {
	Kind    error
	Path    string
	Missing []string
}

func (e *CredentialValidationError) Error() string {
	return CredentialErrorMessage(e)
}

func (e *CredentialValidationError) Unwrap() error { return e.Kind }

// ValidateFirebaseServiceAccount verifies that a local path can be used by
// Firebase without retaining or returning the credential's private contents.
func ValidateFirebaseServiceAccount(credentialsPath string) error {
	credentialsPath = strings.TrimSpace(credentialsPath)
	if credentialsPath == "" {
		return &CredentialValidationError{Kind: ErrCredentialFileMissing, Path: credentialsPath}
	}
	expandedPath := expandPath(credentialsPath)
	info, err := os.Stat(expandedPath)
	if errors.Is(err, os.ErrNotExist) {
		return &CredentialValidationError{Kind: ErrCredentialFileMissing, Path: credentialsPath}
	}
	if err != nil || info.IsDir() {
		return &CredentialValidationError{Kind: ErrCredentialFileUnreadable, Path: credentialsPath}
	}
	content, err := os.ReadFile(expandedPath)
	if err != nil {
		return &CredentialValidationError{Kind: ErrCredentialFileUnreadable, Path: credentialsPath}
	}
	var account struct {
		Type        string `json:"type"`
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		ProjectID   string `json:"project_id"`
	}
	if err := json.Unmarshal(content, &account); err != nil {
		return &CredentialValidationError{Kind: ErrCredentialJSONInvalid, Path: credentialsPath}
	}
	if account.Type != "service_account" {
		return &CredentialValidationError{Kind: ErrCredentialServiceAccount, Path: credentialsPath}
	}
	missing := make([]string, 0, 3)
	for _, field := range []struct{ name, value string }{
		{"client_email", account.ClientEmail},
		{"private_key", account.PrivateKey},
		{"project_id", account.ProjectID},
	} {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return &CredentialValidationError{Kind: ErrCredentialFieldsMissing, Path: credentialsPath, Missing: missing}
	}
	return nil
}

// CredentialErrorMessage is suitable for the CLI. The server must use its
// own stable, path-free response messages when rendering browser errors.
func CredentialErrorMessage(err error) string {
	var validation *CredentialValidationError
	if !errors.As(err, &validation) {
		return "凭据文件无法读取"
	}
	switch {
	case errors.Is(validation, ErrCredentialFileMissing):
		return fmt.Sprintf("凭据文件不存在：%s", validation.Path)
	case errors.Is(validation, ErrCredentialFileUnreadable):
		return fmt.Sprintf("凭据文件无法读取：%s", validation.Path)
	case errors.Is(validation, ErrCredentialJSONInvalid):
		return "凭据文件不是有效的 JSON"
	case errors.Is(validation, ErrCredentialServiceAccount):
		return "凭据文件不是 Firebase 服务账号 JSON（type 必须为 service_account）"
	case errors.Is(validation, ErrCredentialFieldsMissing):
		return "凭据文件缺少字段：" + strings.Join(validation.Missing, "、") + "（需要 Firebase 服务账号 JSON，见 README「连接 Firebase」）"
	default:
		return "凭据文件无法读取"
	}
}

// LoadCredentialReference reads only a filesystem reference. The service
// account JSON itself remains outside the workspace and is never copied here.
func LoadCredentialReference(workspace, environmentID string) (string, error) {
	b, err := os.ReadFile(filepath.Join(workspace, ".conflow", "provider", environmentID+".yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", ErrNotConfigured
	}
	var config struct {
		CredentialsPath string `yaml:"credentials_path"`
	}
	if yaml.Unmarshal(b, &config) != nil || config.CredentialsPath == "" {
		return "", ErrNotConfigured
	}
	return config.CredentialsPath, nil
}

// SaveCredentialReference stores a local path reference under .conflow, which
// is ignored by Git. It never copies service account JSON into the workspace.
func SaveCredentialReference(workspace, environmentID, credentialsPath string) error {
	credentialsPath = strings.TrimSpace(credentialsPath)
	if credentialsPath == "" {
		return ErrNotConfigured
	}
	directory := filepath.Join(workspace, ".conflow", "provider")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	content, err := yaml.Marshal(struct {
		CredentialsPath string `yaml:"credentials_path"`
	}{CredentialsPath: credentialsPath})
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, environmentID+"-*.yaml")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filepath.Join(directory, environmentID+".yaml"))
}

// CredentialPathDisplay intentionally reveals only the final file name.
func CredentialPathDisplay(credentialsPath string) string {
	name := filepath.Base(filepath.Clean(strings.TrimSpace(credentialsPath)))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return ""
	}
	return "…/" + name
}
