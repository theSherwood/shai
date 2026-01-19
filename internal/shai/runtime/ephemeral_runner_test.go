package shai

import (
	"os"
	"path/filepath"
	"testing"

	configpkg "github.com/colony-2/shai/internal/shai/runtime/config"
	"github.com/stretchr/testify/require"
)

func TestBuildBootstrapArgsIncludesResources(t *testing.T) {
	runner := &EphemeralRunner{
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		resources: []*configpkg.ResolvedResource{
			{
				Name: "base",
				Spec: &configpkg.ResourceSet{
					Vars: []configpkg.VarMapping{
						{Source: "TOKEN", Target: "INSIDE_TOKEN"},
					},
					HTTP: []string{"example.com"},
					Ports: []configpkg.Port{
						{Host: "github.com", Port: 443},
					},
				},
			},
		},
		hostEnv: map[string]string{
			"TOKEN": "super-secret",
		},
		config: EphemeralConfig{
			PostSetupExec: &ExecSpec{
				Command: []string{"/bin/bash", "-lc", "echo hi"},
				Env: map[string]string{
					"FOO": "bar",
				},
				UseTTY: false,
			},
			Verbose: true,
		},
	}

	args, err := runner.buildBootstrapArgs()
	require.NoError(t, err)

	require.Equal(t, []string{
		"--version", "1",
		"--user", "shai",
		"--workspace", "/src",
		"--rm", "true",
		"--exec-env", "INSIDE_TOKEN=super-secret",
		"--exec-cmd", "/bin/bash",
		"--exec-cmd", "-lc",
		"--exec-cmd", "echo hi",
		"--exec-env", "FOO=bar",
		"--http-allow", "example.com",
		"--port-allow", "github.com:443",
		"--verbose",
	}, args)
}

func TestBuildBootstrapArgsMissingEnvFails(t *testing.T) {
	runner := &EphemeralRunner{
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		resources: []*configpkg.ResolvedResource{
			{
				Name: "base",
				Spec: &configpkg.ResourceSet{
					Vars: []configpkg.VarMapping{
						{Source: "MISSING", Target: "INSIDE"},
					},
				},
			},
		},
		hostEnv: map[string]string{},
	}

	_, err := runner.buildBootstrapArgs()
	require.Error(t, err)
	require.Contains(t, err.Error(), "host env \"MISSING\" not set")
}

func TestBuildBootstrapArgsIncludesExposedPorts(t *testing.T) {
	runner := &EphemeralRunner{
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		resources: []*configpkg.ResolvedResource{
			{
				Name: "web",
				Spec: &configpkg.ResourceSet{
					Expose: []configpkg.ExposedPort{
						{Host: 8000, Container: 8000, Protocol: "tcp"},
						{Host: 9000, Container: 9090, Protocol: "udp"},
					},
				},
			},
		},
		hostEnv: map[string]string{},
	}

	args, err := runner.buildBootstrapArgs()
	require.NoError(t, err)

	// Verify --expose flags are present with correct format
	require.Contains(t, args, "--expose")
	require.Contains(t, args, "8000:8000/tcp")
	require.Contains(t, args, "9000:9090/udp")
}

func TestChooseImagePrecedence(t *testing.T) {
	img, src := chooseImage("base", "cli-override", "apply-override")
	require.Equal(t, "cli-override", img)
	require.Equal(t, "cli", src)

	img, src = chooseImage("base", "", "apply-override")
	require.Equal(t, "apply-override", img)
	require.Equal(t, "apply", src)

	img, src = chooseImage("base", "  ", "")
	require.Equal(t, "base", img)
	require.Equal(t, "", src)
}

func TestResourceMountsSkipMissingPaths(t *testing.T) {
	tDir := t.TempDir()
	existing := filepath.Join(tDir, "existing")
	err := os.Mkdir(existing, 0o755)
	require.NoError(t, err)

	runner := &EphemeralRunner{
		config: EphemeralConfig{
			WorkingDir: tDir,
		},
		resources: []*configpkg.ResolvedResource{
			{
				Name: "set",
				Spec: &configpkg.ResourceSet{
					Mounts: []configpkg.Mount{
						{Source: "existing", Target: "/existing", Mode: "rw"},
						{Source: "missing", Target: "/missing", Mode: "ro"},
					},
				},
			},
		},
	}

	mounts, err := runner.resourceMounts()
	require.NoError(t, err)
	require.Len(t, mounts, 1)
	require.Equal(t, existing, mounts[0].Source)
	require.Equal(t, "/existing", mounts[0].Target)
	require.False(t, mounts[0].ReadOnly)
}

func TestEffectiveWorkspaceSinglePath(t *testing.T) {
	require.Equal(t, "/src/cmd", effectiveWorkspace("/src", []string{"cmd"}))
	require.Equal(t, "/src", effectiveWorkspace("/src", []string{"."}))
}

func TestEffectiveWorkspaceDefaults(t *testing.T) {
	require.Equal(t, "/src", effectiveWorkspace("/src", nil))
	require.Equal(t, "/src", effectiveWorkspace("/src", []string{"one", "two"}))
	require.Equal(t, "/src", effectiveWorkspace("/src", []string{"/abs"}))
}

func TestEffectiveWorkspaceWithDotPrefix(t *testing.T) {
	require.Equal(t, "/src/cmd", effectiveWorkspace("/src", []string{"./cmd"}))
}

func TestBuildDockerConfigsInjectsHostIDs(t *testing.T) {
	tDir := t.TempDir()
	mountBuilder, err := NewMountBuilder(tDir, nil)
	require.NoError(t, err)

	runner := &EphemeralRunner{
		config: EphemeralConfig{
			WorkingDir: tDir,
		},
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		mountBuilder: mountBuilder,
		image:        "example",
		hostEnv:      map[string]string{},
		hostUID:      "1234",
		hostGID:      "5678",
	}

	cfg, _, err := runner.buildDockerConfigs(false, "sandbox-test")
	require.NoError(t, err)
	require.Contains(t, cfg.Env, "DEV_UID=1234")
	require.Contains(t, cfg.Env, "DEV_GID=5678")
}

func TestCollectExposedPorts(t *testing.T) {
	resources := []*configpkg.ResolvedResource{
		{
			Name: "web",
			Spec: &configpkg.ResourceSet{
				Expose: []configpkg.ExposedPort{
					{Host: 8080, Container: 80, Protocol: "tcp"},
					{Host: 8443, Container: 443, Protocol: "tcp"},
				},
			},
		},
		{
			Name: "dns",
			Spec: &configpkg.ResourceSet{
				Expose: []configpkg.ExposedPort{
					{Host: 53, Container: 53, Protocol: "udp"},
				},
			},
		},
	}

	ports := collectExposedPorts(resources)
	require.Len(t, ports, 3)
}

func TestCollectExposedPortsDeduplicates(t *testing.T) {
	resources := []*configpkg.ResolvedResource{
		{
			Name: "first",
			Spec: &configpkg.ResourceSet{
				Expose: []configpkg.ExposedPort{
					{Host: 8000, Container: 8000, Protocol: "tcp"},
				},
			},
		},
		{
			Name: "second",
			Spec: &configpkg.ResourceSet{
				Expose: []configpkg.ExposedPort{
					{Host: 8000, Container: 8000, Protocol: "tcp"}, // duplicate
					{Host: 9000, Container: 9000, Protocol: "tcp"},
				},
			},
		},
	}

	ports := collectExposedPorts(resources)
	require.Len(t, ports, 2) // Should deduplicate the 8000/tcp
}

func TestCollectExposedPortsAllowsSamePortDifferentProtocol(t *testing.T) {
	resources := []*configpkg.ResolvedResource{
		{
			Name: "both",
			Spec: &configpkg.ResourceSet{
				Expose: []configpkg.ExposedPort{
					{Host: 53, Container: 53, Protocol: "tcp"},
					{Host: 53, Container: 53, Protocol: "udp"},
				},
			},
		},
	}

	ports := collectExposedPorts(resources)
	require.Len(t, ports, 2) // Both should be kept since protocols differ
}

func TestBuildDockerConfigsPortBindings(t *testing.T) {
	tDir := t.TempDir()
	mountBuilder, err := NewMountBuilder(tDir, nil)
	require.NoError(t, err)

	runner := &EphemeralRunner{
		config: EphemeralConfig{
			WorkingDir: tDir,
		},
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		mountBuilder: mountBuilder,
		image:        "example",
		hostEnv:      map[string]string{},
		resources: []*configpkg.ResolvedResource{
			{
				Name: "web",
				Spec: &configpkg.ResourceSet{
					Expose: []configpkg.ExposedPort{
						{Host: 8080, Container: 80, Protocol: "tcp"},
						{Host: 8443, Container: 443, Protocol: "tcp"},
					},
				},
			},
		},
	}

	cfg, hostCfg, err := runner.buildDockerConfigs(false, "sandbox-test")
	require.NoError(t, err)

	// Verify exposed ports in container config
	require.Len(t, cfg.ExposedPorts, 2)

	// Verify port bindings in host config
	require.Len(t, hostCfg.PortBindings, 2)

	// Check 80/tcp maps to host 8080
	bindings80 := hostCfg.PortBindings["80/tcp"]
	require.Len(t, bindings80, 1)
	require.Equal(t, "8080", bindings80[0].HostPort)

	// Check 443/tcp maps to host 8443
	bindings443 := hostCfg.PortBindings["443/tcp"]
	require.Len(t, bindings443, 1)
	require.Equal(t, "8443", bindings443[0].HostPort)
}

func TestBuildDockerConfigsNoPortsWhenEmpty(t *testing.T) {
	tDir := t.TempDir()
	mountBuilder, err := NewMountBuilder(tDir, nil)
	require.NoError(t, err)

	runner := &EphemeralRunner{
		config: EphemeralConfig{
			WorkingDir: tDir,
		},
		shaiConfig: &configpkg.Config{
			User:      "shai",
			Workspace: "/src",
		},
		mountBuilder: mountBuilder,
		image:        "example",
		hostEnv:      map[string]string{},
		resources:    []*configpkg.ResolvedResource{},
	}

	cfg, hostCfg, err := runner.buildDockerConfigs(false, "sandbox-test")
	require.NoError(t, err)

	// Verify no exposed ports when none configured
	require.Nil(t, cfg.ExposedPorts)
	require.Nil(t, hostCfg.PortBindings)
}
