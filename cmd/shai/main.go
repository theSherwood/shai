package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/divisive-ai/vibethis/server/container/pkg/shai"
	"github.com/spf13/cobra"
)

const workspacePath = "/src"

func main() {
	os.Args = normalizeLegacyArgs(os.Args)
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		readWritePaths []string
		configPath     string
		templatePairs  []string
		resourceSets   []string
		imageOverride  string
		containerName  string
		verbose        bool
		noTTY          bool
	)

	cmd := &cobra.Command{
		Use:           "shai [--read-write <path>] [flags] [-- command ...]",
		Short:         "Launch an ephemeral shai sandbox with optional writable mounts",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			varMap, err := parseTemplateVars(templatePairs)
			if err != nil {
				return err
			}

			workingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			var postExec *shai.SandboxExec
			if len(args) > 0 {
				postExec = &shai.SandboxExec{
					Command: args,
					Workdir: workspacePath,
					UseTTY:  !noTTY,
				}
			}

			ctx, cancel := setupSignals()
			defer cancel()

			if err := runEphemeral(ctx, workingDir, readWritePaths, verbose, postExec, configPath, varMap, resourceSets, imageOverride); err != nil {
				return err
			}

			_ = containerName // Flag retained for future naming support.
			return nil
		},
	}

	flags := cmd.Flags()
	flags.StringArrayVar(&readWritePaths, "read-write", nil, "Path to mount read-write (repeatable)")
	flags.StringVarP(&configPath, "config", "c", "", fmt.Sprintf("Path to Shai config (default: <workspace>/%s)", shai.DefaultConfigRelPath))
	flags.StringArrayVar(&resourceSets, "resource-set", nil, "Resource set to activate (repeatable)")
	flags.StringArrayVarP(&templatePairs, "var", "v", nil, fmt.Sprintf("Template variable for %s (key=value)", shai.DefaultConfigRelPath))
	flags.StringVarP(&imageOverride, "image", "i", "", "Override container image (highest precedence)")
	flags.StringVarP(&containerName, "name", "n", "", "Container name (optional)")
	flags.BoolVarP(&verbose, "verbose", "V", false, "Enable verbose logging")
	flags.BoolVarP(&noTTY, "no-tty", "T", false, "Disable TTY for post-setup command")

	return cmd
}

func parseTemplateVars(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	vars := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid var %q (expected key=value)", pair)
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			return nil, fmt.Errorf("invalid var %q (empty key)", pair)
		}
		vars[key] = parts[1]
	}
	return vars, nil
}

func normalizeLegacyArgs(args []string) []string {
	const (
		rwAlias = "-rw"
		rsAlias = "-rs"
	)

	out := make([]string, len(args))
	copy(out, args)

	for i, arg := range out {
		switch {
		case arg == rwAlias:
			out[i] = "--read-write"
		case strings.HasPrefix(arg, rwAlias+"="):
			out[i] = "--read-write" + arg[len(rwAlias):]
		case arg == rsAlias:
			out[i] = "--resource-set"
		case strings.HasPrefix(arg, rsAlias+"="):
			out[i] = "--resource-set" + arg[len(rsAlias):]
		}
	}
	return out
}

func runEphemeral(ctx context.Context, workingDir string, rwPaths []string, verbose bool, postExec *shai.SandboxExec, configPath string, vars map[string]string, resourceSets []string, imageOverride string) error {
	sandbox, err := shai.NewSandbox(shai.SandboxConfig{
		WorkingDir:     workingDir,
		ConfigFile:     configPath,
		TemplateVars:   vars,
		ReadWritePaths: rwPaths,
		ResourceSets:   resourceSets,
		Verbose:        verbose,
		PostSetupExec:  postExec,
		ImageOverride:  imageOverride,
	})
	if err != nil {
		return err
	}
	defer sandbox.Close()

	return sandbox.Run(ctx)
}

// setupSignals configures signal handling and returns a cancellable context.
// In ephemeral mode, SIGINT is ignored so Ctrl-C reaches the container shell.
func setupSignals() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Ignore(syscall.SIGINT)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()
	return ctx, cancel
}
