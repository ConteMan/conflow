package operation

import (
	"errors"
	"testing"
)

func TestSpec009ReadOperationStagesAndRemoteState(t *testing.T) {
	for _, test := range []struct {
		typ    string
		stages []string
	}{
		{"remote_pull", []string{"reading_remote", "snapshotting", "completed"}},
		{"remote_validate", []string{"validating_remote", "completed"}},
		{"plan", []string{"reading_remote", "compiling", "analyzing", "completed"}},
	} {
		t.Run(test.typ, func(t *testing.T) {
			store, err := Open("")
			if err != nil {
				t.Fatal(err)
			}
			op, err := store.Create(test.typ)
			if err != nil {
				t.Fatal(err)
			}
			for _, stage := range test.stages {
				op, err = store.Update(op.OperationID, "running", stage, nil, nil, "unchanged")
				if err != nil {
					t.Fatal(err)
				}
				if op.RemoteState != "unchanged" {
					t.Fatalf("remote_state=%s", op.RemoteState)
				}
			}
			if _, err := store.Update(op.OperationID, "running", "submitting", nil, nil, "unchanged"); !errors.Is(err, ErrInvalidStage) {
				t.Fatalf("invalid stage error=%v", err)
			}
		})
	}
}
