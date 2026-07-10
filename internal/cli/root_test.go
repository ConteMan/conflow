package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateListenAddress(t *testing.T) {
	for _, address := range []string{"127.0.0.1:9010", "[::1]:9010", "localhost:9010"} {
		if err := validateListenAddress(address); err != nil {
			t.Fatalf("validateListenAddress(%q) error = %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:9010", ":9010", "192.168.1.10:9010"} {
		if err := validateListenAddress(address); err == nil {
			t.Fatalf("validateListenAddress(%q) error = nil", address)
		}
	}
}

func TestInitAndValidateCommands(t *testing.T) {
	workspace := t.TempDir()

	initOutput := &bytes.Buffer{}
	initCommand := New("test")
	initCommand.SetOut(initOutput)
	initCommand.SetArgs([]string{"init", "--dir", workspace})
	if err := initCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(initOutput.String(), ".conflow/project.yaml") {
		t.Fatalf("init output = %q", initOutput.String())
	}

	validateOutput := &bytes.Buffer{}
	validateCommand := New("test")
	validateCommand.SetOut(validateOutput)
	validateCommand.SetArgs([]string{"validate", "--workspace", workspace})
	if err := validateCommand.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := validateOutput.String(); got != "validated photo-editor with 2 environments\n" {
		t.Fatalf("validate output = %q", got)
	}
}
