package shai

import (
	"testing"

	configpkg "github.com/divisive-ai/vibethis/server/container/internal/shai/runtime/config"
	"github.com/stretchr/testify/require"
)

func TestBuildBootstrapArgsIncludesResources(t *testing.T) {
	runner := &EphemeralRunner{
		shaiConfig: &configpkg.Config{
			User:      "devuser",
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
		"--user", "devuser",
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
			User:      "devuser",
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
