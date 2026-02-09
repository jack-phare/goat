package agent

import "testing"

func TestLoopState_RecordFileAccess(t *testing.T) {
	state := &LoopState{}

	// First access initializes the map
	state.RecordFileAccess("/tmp/foo.go", "read")
	if state.AccessedFiles == nil {
		t.Fatal("AccessedFiles should be initialized")
	}
	if !state.AccessedFiles["/tmp/foo.go"]["read"] {
		t.Error("expected read op on /tmp/foo.go")
	}

	// Multiple ops on the same file
	state.RecordFileAccess("/tmp/foo.go", "write")
	if !state.AccessedFiles["/tmp/foo.go"]["read"] {
		t.Error("read op should still be present")
	}
	if !state.AccessedFiles["/tmp/foo.go"]["write"] {
		t.Error("write op should be present")
	}

	// Different file
	state.RecordFileAccess("/tmp/bar.go", "edit")
	if len(state.AccessedFiles) != 2 {
		t.Errorf("expected 2 files, got %d", len(state.AccessedFiles))
	}
}

func TestLoopState_RecordFileAccess_NilMap(t *testing.T) {
	state := &LoopState{}

	// Should not panic with nil initial map
	state.RecordFileAccess("/a/b/c", "glob")
	if !state.AccessedFiles["/a/b/c"]["glob"] {
		t.Error("expected glob op")
	}
}
