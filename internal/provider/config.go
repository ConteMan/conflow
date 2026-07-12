package provider

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

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
