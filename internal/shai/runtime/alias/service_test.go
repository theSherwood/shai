package alias

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaybeStartInitializesWithoutEntries(t *testing.T) {
	svc, err := MaybeStart(Config{
		WorkingDir: t.TempDir(),
		ShellPath:  "/bin/sh",
	})
	require.NoError(t, err)
	require.NotNil(t, svc)
	t.Cleanup(func() {
		svc.Close()
	})

	env := svc.Env()
	require.NotEmpty(t, env)

	var endpoint string
	for _, kv := range env {
		if strings.HasPrefix(kv, "SHAI_ALIAS_ENDPOINT=") {
			endpoint = kv
			break
		}
	}
	require.NotEmpty(t, endpoint, "expected endpoint env var")
}
