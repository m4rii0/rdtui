package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestPrintVersionPropagatesWriteError(t *testing.T) {
	want := errors.New("output unavailable")
	if err := printVersion(failingWriter{err: want}); !errors.Is(err, want) {
		t.Fatalf("printVersion error = %v, want %v", err, want)
	}
}

func TestRunReturnsErrorWhenVersionOutputFails(t *testing.T) {
	var stderr bytes.Buffer
	want := errors.New("output unavailable")
	if code := run([]string{"--version"}, failingWriter{err: want}, &stderr); code != 1 {
		t.Fatalf("run exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), want.Error()) {
		t.Fatalf("stderr = %q, want output error", stderr.String())
	}
}

func TestRunPreservesUsageExitCodeWhenStderrFails(t *testing.T) {
	var stdout bytes.Buffer
	stderr := failingWriter{err: errors.New("stderr unavailable")}
	if code := run([]string{"update", "--unknown"}, &stdout, stderr); code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
}
