package provider

import (
	"errors"
	"os"
	"path/filepath"

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
