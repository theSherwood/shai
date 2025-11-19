package alias

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestExecutorRunSuccess(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hello.sh")
	writeExecutable(t, script, "#!/bin/sh\necho \"HELLO:$*\"\n")

	entry, err := NewEntry("hello", "", script, ".*")
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}

	executor := &Executor{
		WorkingDir: dir,
		ShellPath:  shellPath(),
		Timeout:    2 * time.Second,
	}

	var stdout bytes.Buffer
	res, err := executor.Run(context.Background(), entry, []string{"one", "two"}, Streams{Stdout: &stdout})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
	if got := stdout.String(); got != "HELLO:one two\n" {
		t.Fatalf("unexpected stdout: %q", got)
	}
}

func TestExecutorRunTimeout(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "sleep.sh")
	writeExecutable(t, script, "#!/bin/sh\nsleep 2\n")

	entry, err := NewEntry("sleepy", "", script, "")
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}

	executor := &Executor{
		WorkingDir: dir,
		ShellPath:  shellPath(),
		Timeout:    200 * time.Millisecond,
	}

	if _, err := executor.Run(context.Background(), entry, nil, Streams{}); err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestExecutorRunRejectsArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "args.sh")
	writeExecutable(t, script, "#!/bin/sh\necho \"$*\"\n")

	entry, err := NewEntry("restricted", "", script, "^--msg=[a-z]+$")
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}

	executor := &Executor{
		WorkingDir: dir,
		ShellPath:  shellPath(),
		Timeout:    time.Second,
	}

	if _, err := executor.Run(context.Background(), entry, []string{"--msg=BadValue"}, Streams{}); err == nil {
		t.Fatalf("expected argument validation error")
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
}

func shellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	if runtime.GOOS == "windows" {
		return "C:\\Windows\\System32\\wsl.exe"
	}
	return "/bin/sh"
}
