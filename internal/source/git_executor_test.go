package source

import (
	"context"
	"reflect"
	"testing"
)

type recordingRunner struct {
	name string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args []string) ([]byte, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return []byte("ok\n"), nil
}

func TestGitExecutorPreservesArgumentBoundaries(t *testing.T) {
	runner := &recordingRunner{}
	executor := GitExecutor{Runner: runner}
	if _, err := executor.Output(context.Background(), "/tmp/work space", "add", "--", "config/a; echo unsafe.json"); err != nil {
		t.Fatal(err)
	}
	if runner.name != "git" {
		t.Fatalf("name = %q", runner.name)
	}
	want := []string{"-C", "/tmp/work space", "add", "--", "config/a; echo unsafe.json"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("args = %#v, want %#v", runner.args, want)
	}
}
