package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFirebasePullAndValidateUsesMemoryOnlyCredentials(t *testing.T) {
	var sawAuthorization, sawValidate bool
	server := newFakeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil || !strings.Contains(r.Form.Get("assertion"), ".") {
				t.Fatalf("missing JWT assertion")
			}
			_, _ = w.Write([]byte(`{"access_token":"test-access-token"}`))
		case "/v1/projects/test-project/remoteConfig":
			sawAuthorization = r.Header.Get("Authorization") == "Bearer test-access-token"
			if r.Method == http.MethodPut {
				sawValidate = r.URL.Query().Get("validate_only") == "true"
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("ETag", "test-etag")
			_, _ = w.Write([]byte(`{"version":{"versionNumber":"7"},"parameters":{"ad_frequency_cap":{"defaultValue":{"value":"30000"}}}}`))
		default:
			t.Fatalf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer server.Close()
	credential := writeServiceAccount(t, server.URL+"/token")
	client := NewFirebase(FirebaseConfig{ProjectID: "test-project", CredentialsPath: credential, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	template, err := client.Pull(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if template.ETag != "test-etag" || template.Version != "7" || !sawAuthorization {
		t.Fatalf("template=%#v auth=%t", template, sawAuthorization)
	}
	if err := client.Validate(context.Background(), []byte(`{"parameters":{}}`)); err != nil {
		t.Fatal(err)
	}
	if !sawValidate {
		t.Fatal("validate_only query was not sent")
	}
	for _, err := range []error{SafeError(errors.New("Bearer test-access-token")), SafeError(errors.New("private_key test-private-key"))} {
		if strings.Contains(err.Error(), "test-access-token") || strings.Contains(err.Error(), "test-private-key") {
			t.Fatalf("secret leaked in error %q", err)
		}
	}
}

func TestFirebaseMapsAuthClientServerTimeoutAndCancellation(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		cancel  bool
		want    error
	}{
		{"auth", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }, false, ErrUnauthorized},
		{"server", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }, false, ErrUnavailable},
		{"cancelled", func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, true, context.Canceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newFakeServer(t, test.handler)
			defer server.Close()
			credential := writeServiceAccount(t, server.URL)
			client := NewFirebase(FirebaseConfig{ProjectID: "test-project", CredentialsPath: credential, BaseURL: server.URL, HTTPClient: server.Client()})
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancel {
				cancel()
			}
			defer cancel()
			_, err := client.Pull(ctx)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want %v", err, test.want)
			}
		})
	}
}

func TestFirebaseValidateMapsClientErrorAndTimeout(t *testing.T) {
	server := newFakeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-access-token"}`))
			return
		}
		if r.URL.Query().Get("validate_only") == "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		time.Sleep(30 * time.Millisecond)
	}))
	defer server.Close()
	credential := writeServiceAccount(t, server.URL+"/token")
	client := NewFirebase(FirebaseConfig{ProjectID: "test-project", CredentialsPath: credential, BaseURL: server.URL + "/v1", HTTPClient: &http.Client{Timeout: 5 * time.Millisecond}})
	if err := client.Validate(context.Background(), []byte(`{"parameters":{}}`)); !errors.Is(err, ErrValidation) {
		t.Fatalf("4xx validation error=%v", err)
	}
	_, err := client.Pull(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout error=%v", err)
	}
}

func TestFirebasePublishUsesIfMatchAndMapsConflict(t *testing.T) {
	var receivedIfMatch string
	server := newFakeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-access-token"}`))
			return
		}
		if r.Method != http.MethodPut {
			t.Fatalf("method=%s", r.Method)
		}
		receivedIfMatch = r.Header.Get("If-Match")
		if receivedIfMatch == "stale" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		w.Header().Set("ETag", "after")
		_, _ = w.Write([]byte(`{"version":{"versionNumber":"8"},"parameters":{}}`))
	}))
	defer server.Close()
	credential := writeServiceAccount(t, server.URL+"/token")
	client := NewFirebase(FirebaseConfig{ProjectID: "test-project", CredentialsPath: credential, BaseURL: server.URL + "/v1", HTTPClient: server.Client()})
	published, err := client.Publish(context.Background(), []byte(`{"parameters":{}}`), "before")
	if err != nil || receivedIfMatch != "before" || published.ETag != "after" || published.Version != "8" {
		t.Fatalf("published=%#v if-match=%q err=%v", published, receivedIfMatch, err)
	}
	_, err = client.Publish(context.Background(), []byte(`{"parameters":{}}`), "stale")
	if !errors.Is(err, ErrETagMismatch) {
		t.Fatalf("conflict=%v", err)
	}
}

func newFakeServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("httptest listener unavailable in this sandbox: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func writeServiceAccount(t *testing.T, tokenURL string) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	document, _ := json.Marshal(map[string]string{"client_email": "test@example.invalid", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": tokenURL})
	path := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(path, document, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
