package shai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/colony-2/shai/internal/shai/runtime/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderedResourcePaths(t *testing.T) {
	paths := orderedResourcePaths([]string{"src", ".", "src", "pkg"})
	require.Equal(t, []string{".", "src", "pkg"}, paths)
}

func TestCallEntriesFromResources(t *testing.T) {
	resources := []*config.ResolvedResource{
		{
			Name: "global",
			Spec: &config.ResourceSet{
				Calls: []config.Call{
					{Name: "git-sync", Command: "git pull --rebase"},
				},
			},
		},
		{
			Name: "feature",
			Spec: &config.ResourceSet{
				Calls: []config.Call{
					{Name: "git-sync", Command: "git pull"},
					{Name: "deploy", Command: "./scripts/deploy.sh", AllowedArgs: "^--env=(dev|prod)$"},
				},
			},
		},
	}

	entries, err := callEntriesFromResources(resources)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
		if e.Name == "deploy" {
			assert.NoError(t, e.ValidateArgs("--env=dev"))
			assert.Error(t, e.ValidateArgs("--env=qa"))
		}
	}
	assert.ElementsMatch(t, []string{"git-sync", "deploy"}, names)
}

func TestResolvedResourcesWithExtraSets(t *testing.T) {
	cfg := loadTestConfig(t, `
type: shai-sandbox
version: 1
image: example
user: dev
workspace: /src
resources:
  base:
    vars:
      - source: FOO
        target: BAR
  opt:
    http: ["example.com"]
  another: {}
apply:
  - path: ./
    resources: [base]
`)

	resources, names, image, err := resolvedResources(cfg, []string{"."}, []string{"opt", "base", "another"})
	require.NoError(t, err)
	require.Equal(t, []string{"opt", "base", "another"}, names)
	require.Len(t, resources, 3)
	assert.Equal(t, "", image)
}

func TestResolvedResourcesUnknownSet(t *testing.T) {
	cfg := loadTestConfig(t, `
type: shai-sandbox
version: 1
image: example
user: dev
workspace: /src
resources:
  base: {}
apply:
  - path: ./
    resources: [base]
`)

	_, _, _, err := resolvedResources(cfg, []string{"."}, []string{"missing", "base"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestResolvedResourcesImageOverrideByOverlay(t *testing.T) {
	cfg := loadTestConfig(t, `
type: shai-sandbox
version: 1
image: example
user: dev
workspace: /src
resources:
  base: {}
apply:
  - path: ./
    resources: [base]
  - path: ./foo
    resources: [base]
    image: foo-image
  - path: ./bar
    resources: [base]
    image: bar-image
`)

	_, _, image, err := resolvedResources(cfg, []string{"bar", "foo"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "bar-image", image)

	_, _, image, err = resolvedResources(cfg, []string{"foo", "bar"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "foo-image", image)
}

func TestResolvedResourcesImageOverridePrefersSpecificPath(t *testing.T) {
	cfg := loadTestConfig(t, `
type: shai-sandbox
version: 1
image: example
user: dev
workspace: /src
resources:
  base: {}
apply:
  - path: ./
    resources: [base]
  - path: ./bar
    resources: [base]
    image: bar-image
  - path: ./bar/baz
    resources: [base]
    image: baz-image
`)

	_, _, image, err := resolvedResources(cfg, []string{"bar/baz"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "baz-image", image)

	_, _, image, err = resolvedResources(cfg, []string{"bar/qux"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "bar-image", image)
}

func TestResolvedResourcesWithExposedPorts(t *testing.T) {
	cfg := loadTestConfig(t, `
type: shai-sandbox
version: 1
image: example
user: dev
workspace: /src
resources:
  web:
    expose:
      - 8000
      - 8443
  api:
    expose:
      - host: 3000
        container: 3000
        protocol: tcp
  database:
    expose:
      - host: 5432
        container: 5432
apply:
  - path: ./
    resources: [web]
  - path: ./services
    resources: [web, api]
`)

	// Test ports from apply rules at root path
	resources, names, _, err := resolvedResources(cfg, []string{"."}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"web"}, names)
	require.Len(t, resources, 1)

	ports := collectExposedPorts(resources)
	require.Len(t, ports, 2) // 8000, 8443
	require.Equal(t, 8000, ports[0].Host)
	require.Equal(t, 8443, ports[1].Host)

	// Test ports from apply rules at subpath (combines web + api)
	resources, names, _, err = resolvedResources(cfg, []string{"services"}, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"web", "api"}, names)
	require.Len(t, resources, 2)

	ports = collectExposedPorts(resources)
	require.Len(t, ports, 3) // 8000, 8443, 3000

	// Test CLI --resource-set flag adds extra ports
	resources, names, _, err = resolvedResources(cfg, []string{"."}, []string{"database"})
	require.NoError(t, err)
	require.Equal(t, []string{"database", "web"}, names)
	require.Len(t, resources, 2)

	ports = collectExposedPorts(resources)
	require.Len(t, ports, 3) // 5432, 8000, 8443

	// Test CLI --resource-set overrides order (CLI first, then apply rules)
	resources, names, _, err = resolvedResources(cfg, []string{"services"}, []string{"database", "api"})
	require.NoError(t, err)
	require.Equal(t, []string{"database", "api", "web"}, names) // CLI first, then apply
	require.Len(t, resources, 3)

	ports = collectExposedPorts(resources)
	require.Len(t, ports, 4) // 5432, 3000, 8000, 8443
}

func loadTestConfig(t *testing.T, contents string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".shai", "config.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644))
	cfg, err := config.Load(path, map[string]string{"FOO": "bar"}, map[string]string{})
	require.NoError(t, err)
	return cfg
}
