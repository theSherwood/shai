package shai

import (
	"path/filepath"
	"testing"
)

func TestLoadSandboxConfigDefaults(t *testing.T) {
	cfg, err := LoadSandboxConfig("")
	if err != nil {
		t.Fatalf("LoadSandboxConfig errored: %v", err)
	}
	if cfg.WorkingDir == "" {
		t.Fatalf("expected working dir to be populated")
	}
	wantConfig := filepath.Join(cfg.WorkingDir, DefaultConfigRelPath)
	if cfg.ConfigFile != wantConfig {
		t.Fatalf("expected default config file %q, got %q", wantConfig, cfg.ConfigFile)
	}
}

func TestRuntimeConfigConversion(t *testing.T) {
	cfg := SandboxConfig{
		WorkingDir: "/workspace",
		ConfigFile: "/workspace/" + DefaultConfigRelPath,
		ReadWritePaths: []string{
			".",
		},
		PostSetupExec: &SandboxExec{
			Command: []string{"echo", "hi"},
			Env: map[string]string{
				"FOO": "bar",
			},
			Workdir: "/src",
			UseTTY:  true,
		},
	}

	rc := cfg.runtimeConfig()
	if rc.WorkingDir != cfg.WorkingDir {
		t.Fatalf("runtime working dir mismatch: %q != %q", rc.WorkingDir, cfg.WorkingDir)
	}
	if rc.PostSetupExec == nil {
		t.Fatalf("expected post setup exec to be copied")
	}
	if got, want := rc.PostSetupExec.Command, cfg.PostSetupExec.Command; len(got) != len(want) {
		t.Fatalf("command mismatch, got %v want %v", got, want)
	}
	if rc.PostSetupExec.UseTTY != cfg.PostSetupExec.UseTTY {
		t.Fatalf("useTTY mismatch")
	}
}
