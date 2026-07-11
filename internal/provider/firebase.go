package provider

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const firebaseScope = "https://www.googleapis.com/auth/firebase.remoteconfig"

type FirebaseConfig struct {
	ProjectID       string
	CredentialsPath string
	BaseURL         string // Test-only override; production uses the Firebase REST origin.
	HTTPClient      *http.Client
}

type Firebase struct{ config FirebaseConfig }

func NewFirebase(config FirebaseConfig) *Firebase {
	if config.BaseURL == "" {
		config.BaseURL = "https://firebaseremoteconfig.googleapis.com/v1"
	}
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Firebase{config: config}
}

func (f *Firebase) Capabilities() Capabilities {
	return Capabilities{Pull: true, Validate: true, Publish: true, Rollback: true}
}
func (f *Firebase) Status(context.Context) Status {
	if strings.TrimSpace(f.config.CredentialsPath) == "" || strings.TrimSpace(f.config.ProjectID) == "" {
		return Status{Status: "not_configured", Capabilities: f.Capabilities()}
	}
	if _, err := os.Stat(expandPath(f.config.CredentialsPath)); err != nil {
		return Status{Status: "unavailable", Capabilities: f.Capabilities()}
	}
	return Status{Status: "connected", Capabilities: f.Capabilities()}
}
func (f *Firebase) Connect(ctx context.Context) error {
	_, err := f.accessToken(ctx)
	return SafeError(err)
}

func (f *Firebase) Pull(ctx context.Context) (Template, error) {
	return f.pull(ctx, "")
}

// PullVersion performs a read-only fetch of a historical Firebase template.
// Firebase accepts the version selector on the same Remote Config resource.
func (f *Firebase) PullVersion(ctx context.Context, version string) (Template, error) {
	if strings.TrimSpace(version) == "" {
		return Template{}, ErrValidation
	}
	return f.pull(ctx, version)
}

func (f *Firebase) pull(ctx context.Context, version string) (Template, error) {
	token, err := f.accessToken(ctx)
	if err != nil {
		return Template{}, SafeError(err)
	}
	u := f.templateURL()
	if version != "" {
		parsed, _ := url.Parse(u)
		query := parsed.Query()
		query.Set("version_number", version)
		parsed.RawQuery = query.Encode()
		u = parsed.String()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Template{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := f.config.HTTPClient.Do(req)
	if err != nil {
		return Template{}, SafeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Template{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Template{}, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Template{}, ErrUnavailable
	}
	if !json.Valid(body) {
		return Template{}, ErrUnavailable
	}
	var metadata struct {
		Version struct {
			VersionNumber string `json:"versionNumber"`
		} `json:"version"`
	}
	_ = json.Unmarshal(body, &metadata)
	return Template{Raw: body, ETag: resp.Header.Get("ETag"), Version: metadata.Version.VersionNumber, ObservedAt: time.Now().UTC()}, nil
}

// ListVersions is intentionally metadata-only. The Remote Config REST API
// has no secret-bearing version endpoint; this query is safe to expose only
// through application orchestration.
func (f *Firebase) ListVersions(ctx context.Context) ([]Version, error) {
	template, err := f.Pull(ctx)
	if err != nil {
		return nil, err
	}
	return []Version{{Version: template.Version, CreatedAt: template.ObservedAt}}, nil
}

func (f *Firebase) Validate(ctx context.Context, input []byte) error {
	if !json.Valid(input) {
		return ErrValidation
	}
	token, err := f.accessToken(ctx)
	if err != nil {
		return SafeError(err)
	}
	u := f.templateURL()
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	q.Set("validate_only", "true")
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, parsed.String(), strings.NewReader(string(input)))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.config.HTTPClient.Do(req)
	if err != nil {
		return SafeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return ErrValidation
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	return nil
}

// Publish performs the only destructive Firebase operation. The caller must
// have already compared the current ETag; If-Match closes the race afterwards.
func (f *Firebase) Publish(ctx context.Context, input []byte, expectedETag string) (Template, error) {
	if !json.Valid(input) || strings.TrimSpace(expectedETag) == "" {
		return Template{}, ErrValidation
	}
	token, err := f.accessToken(ctx)
	if err != nil {
		return Template{}, SafeError(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, f.templateURL(), strings.NewReader(string(input)))
	if err != nil {
		return Template{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", expectedETag)
	resp, err := f.config.HTTPClient.Do(req)
	if err != nil {
		return Template{}, SafeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Template{}, ErrUnauthorized
	}
	if resp.StatusCode == http.StatusPreconditionFailed {
		return Template{}, ErrETagMismatch
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return Template{}, ErrValidation
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Template{}, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil || !json.Valid(body) {
		return Template{}, ErrUnavailable
	}
	var metadata struct {
		Version struct {
			VersionNumber string `json:"versionNumber"`
		} `json:"version"`
	}
	_ = json.Unmarshal(body, &metadata)
	return Template{Raw: body, ETag: resp.Header.Get("ETag"), Version: metadata.Version.VersionNumber, ObservedAt: time.Now().UTC()}, nil
}

func (f *Firebase) templateURL() string {
	return strings.TrimRight(f.config.BaseURL, "/") + "/projects/" + url.PathEscape(f.config.ProjectID) + "/remoteConfig"
}

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// accessToken deliberately keeps the private key, assertion, and access token
// in local variables only. None are persisted, returned, or attached to errors.
func (f *Firebase) accessToken(ctx context.Context) (string, error) {
	if f.config.CredentialsPath == "" || f.config.ProjectID == "" {
		return "", ErrNotConfigured
	}
	b, err := os.ReadFile(expandPath(f.config.CredentialsPath))
	if err != nil {
		return "", ErrNotConfigured
	}
	var account serviceAccount
	if json.Unmarshal(b, &account) != nil || account.ClientEmail == "" || account.PrivateKey == "" || account.TokenURI == "" {
		return "", ErrNotConfigured
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return "", ErrNotConfigured
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", ErrNotConfigured
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", ErrNotConfigured
	}
	now := time.Now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": account.ClientEmail, "scope": firebaseScope, "aud": account.TokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", ErrNotConfigured
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.config.HTTPClient.Do(req)
	if err != nil {
		return "", SafeError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", ErrUnavailable
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&token); err != nil || token.AccessToken == "" {
		return "", ErrUnavailable
	}
	return token.AccessToken, nil
}

func expandPath(value string) string {
	value = os.ExpandEnv(value)
	if strings.HasPrefix(value, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return path.Join(home, value[2:])
		}
	}
	return value
}

func (f *Firebase) String() string { return fmt.Sprintf("firebase(%s)", f.config.ProjectID) }
