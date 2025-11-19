# shai - a sandboxing shell for ai coding agents

Shai (pronounced "shy") is a sandboxing tool for running CLI-based AI agents inside containers. At it's core, running `shai` will place your terminal inside a container mounted at the current path. This container mounts a read-only copy of your current path at /src as a non-root user and restricts network access to a select list of http and https destinations. All other network traffic is blocked. This is the perfect sandbox for running CLI agents like [Codex](https://github.com/openai/codex), [Claude Code](https://github.com/anthropics/claude-code), [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Cline](https://github.com/cline/cline), [OpenHands](https://github.com/OpenHands/OpenHands), etc. 

## Cellular Software Development & Target Paths
Shai is built around the concept of cellular software development. In this model, agents are given constrained access to individual components as opposed to cross-repo access. They can consume and understand related code but must limit changes to individual components. When Shai is started, a user specifies the specific subdirectory (or subdirectories) that the session will be limited to. This is done via the -rw (or --read-write) flag. For example, if you wanted to give an agent write access to the `agents/research` directory, you would run: `shai -rw agents/research`. This mounts that subdirectory inside the container at `/src/agents/research` while mounting the rest of the workspace read-only.

## Resource Sets
It is often the case that coding agents should have access to different sets of resources depending on context. For example, a Rust application development agent shouldn't have access to production deployment credentials while a Pulumi deployment module built using shouldn't have access to a Rust compiler or arbitrary Github repositories. These collections of resources are called Resource Sets in `shai`. You can define an arbitrary number of named resource sets. Resource sets contain each of the following:
- Valid HTTP/HTTPS destinations
- Host mounts 
- IP Ports
- Environment variables
- Remote Calls

## Apply Rules
Shai allows you to define a set of rules that define which resource sets and container images are applied to which target paths. This allows you to define a set of default resource sets that apply to all subdirectories, and then enhance them for specific subtrees. 

## Selective Elevation - Calls
Sometimes, agents need to perform operations that are outside the scope of their containerized environment. For example, an agent working on embedded firmware may need to be able to flash that code to a specific host-mounted development board. Shai allows you to define specific host-side commands that can be called from inside the container. These remote calls are defined in the `calls` section of a resource set.

## CLI Quick Start
1. Install/build the CLI:
   ```bash
   # MacOS
   brew install shai
   # OR
   npm install -g @colony2/shai
   ```
2. Create `.shai/config.yaml` at your repo root (see the reference below) or rely on the embedded default.
3. Run Shai from your workspace:
   ```bash
   # minimal invocation – read-only workspace sandbox
   shai

   # mark specific directories as writable
   shai -rw app/component1

   # run a command after bootstrap
   shai -rw app/component1 -- codex --dangerously-bypass-approvals-and-sandbox

   # inspect generated bootstrap script and resource decisions
   shai -rw app/component1 src --verbose

   # override the Docker image explicitly
   shai -rw app/component1 --image ghcr.io/example/dev:latest
   ```

### Useful flags
- `--read-write, -rw <path>` (repeatable) – declare relative paths that receive writable overlays inside `/src`; omit to keep the workspace entirely read-only.
- `--resource-set, -rs <name>` – opt into additional resource sets beyond the config’s apply rules.
- `--image, -i <image>` – override the container image; this takes precedence over apply-rule overrides.
- `--var, -v KEY=value` – provide template variables consumed by `${{ vars.KEY }}` expressions.
- `--verbose, -V` – dump bootstrap details.
- `--no-tty, -T` – disable TTY allocation for the post-setup command (structured log mode).

If you pass `-- command ...`, those arguments become the `PostSetupExec` inside the container. Without a command, Shai switches to the configured user and drops you into an interactive login shell.

## `.shai/config.yaml` Reference
Shai automatically loads `<workspace>/.shai/config.yaml` unless `--config` overrides it. Missing configs fall back to the embedded default, which primarily enables a permissive HTTP allow-list and open-source registries.

### Top-level keys
| Key | Required | Description |
| --- | --- | --- |
| `type` | yes | Must be `shai-sandbox`. Used for schema validation.
| `version` | yes | Must be `1`. Future releases may bump this.
| `image` | yes | Base container image. Can use templates.
| `user` | yes | Container user Shai switches to before running your command.
| `workspace` | yes | Absolute path of the repository inside the container (usually `/src`).
| `resources` | yes | Map of resource-set definitions (see below).
| `apply` | yes | Ordered list that maps workspace paths to resource sets and optional image overrides.

### Resource sets
```yaml
resources:
  my-tools:
    vars:
      - source: ${{ env.OPENAI_API_KEY }}
        target: OPENAI_API_KEY
    mounts:
      - source: ~/.cache/model
        target: /var/cache/model
        mode: rw
    calls:
      - name: fetch-secrets
        description: Fetch secrets from the host vault
        command: /usr/local/bin/fetch-secrets.sh
        allowed-args: '^--env=\w+$'
    http:
      - api.openai.com
      - github.com
    ports:
      - host: github.com
        port: 22
```
- `vars` – Copies values from host environment variables (`source`) into container variables (`target`). Missing env references cause load failures.
- `mounts` – Bind mount host paths into the container. `mode` defaults to `ro`; valid values are `ro` or `rw`.
- `calls` – Expose curated host commands inside the sandbox. Names must be unique per path, `command` is executed on the host, and `allowed-args` (optional) is a regex that filters arguments forwarded from inside the container.
- `http` – Hostnames the sandbox is allowed to reach. Use this to tighten egress beyond the defaults.
- `ports` – Explicit host/port pairs that Shai proxies so agents can reach ssh servers or custom endpoints.

### Apply rules
```yaml
apply:
  - path: ./
    resources: [base-allowlist]
  - path: agents/research
    resources: [my-tools, gpu-mount]
    image: ghcr.io/my-org/gpu:latest
```
- `path` – Workspace-relative path that activates resource sets for itself and its subdirectories. `./` (or `.`) matches everything.
- `resources` – Names of resource sets applied by this rule. Within a single path, resource-set calls must define unique `name` values.
- `image` – Optional override that replaces the base image when Shai is invoked from the matching subtree. The root (`.`) rule cannot override the image; use more specific paths.

Rules are evaluated top to bottom. When resolving a workspace path, Shai aggregates all matching resource sets (deduplicated) and selects the most specific image override. CLI `--resource-set` flags append to the resolved list.

### Template expansion
Any string field can embed `${{ env.NAME }}` (host environment variable) or `${{ vars.NAME }}` (provided via `--var` or the Go API). Missing references cause config loading to fail so you catch mistakes early.

### Example
```yaml
# .shai/config.yaml
type: shai-sandbox
version: 1
image: ghcr.io/example/shai-base:latest
user: app
workspace: /src
resources:
  base-allowlist:
    http: [openai.com, api.openai.com]
  agent-dev:
    vars:
      - source: ${{ env.OPENAI_API_KEY }}
        target: OPENAI_API_KEY
    mounts:
      - source: ~/.cache/agents
        target: /var/cache/agents
        mode: rw
    calls:
      - name: refresh-plan
        description: Refresh cached plans on the host
        command: ${{ env.REPO_ROOT }}/scripts/refresh-plan.sh
    ports:
      - host: github.com
        port: 22
apply:
  - path: ./
    resources: [base-allowlist]
  - path: agents
    resources: [agent-dev]
```

## How it works
Shai builds on top of Docker and Docker-compatible daemons. Shai starts an ephemeral container. In this container it sets an entrypoint of a bootstrap script mounted by shai. This bootstrap script sets up additional sandboxing beyond what the base container provides including defining firewalls rules via iptables and setting up a reverse proxy and dns server to restrict external access. Shai also starts a host-side MCP server that is accessible via injected credentials in the container, allowing container access to the remote calls defined in the config. Once the sandbox envionrment is setup, Shai exec's as the provided user command as a non-privileged user.

## Inspiration & Prior Art
The first version of Shai was built on top of [devcontainers](https://containers.dev) (The original underlying library can be [seen here](https://github.com/colony-2/devcontainer-go). Devcontainers are a great and were a good place for Shai to start. However, over time, the deisgn goal differences between Shai and Devcontainers became challenging and thus we ultimately decided to define an alternative configuration:
- the devcontainer spec is not designed for segmented configuration (e.g. subdirectory one gets different patterns than subdirectory two).
- devcontainers are expected to be longer-lived. the way features work assumes this (features can take 30s to many minutes to install). using features in shai meant every new session had a large startup wait.
- there are a lot of features in devcontainers. shai only needed a small subset of them.
- devcontainers doesn't really have any concept of sandboxing controls for things like firewalling, etc. you can define these things in a feature or in the container image, but they're basically diy.
- the devcontainers tools are not really built for throw-away ephemeral containers
- devcontainers don't actively mitigate again things like inclusion of credentials in configuration. (Shai does by doing things like only allowing mapping of ENV variables as opposed to declaration of them directly in config.)

Shai has no desire to replace devcontainers. They are focused on two different use cases.


## Embedding the Go API
Advanced users can embed Shai inside Go binaries using `pkg/shai`:
```go
import (
    "context"
    shai "github.com/divisive-ai/vibethis/server/container/pkg/shai"
)

func runAgent(ctx context.Context, repoRoot string) error {
    cfg, err := shai.LoadSandboxConfig(repoRoot,
        shai.WithReadWritePaths([]string{"agents/cache"}),
        shai.WithResourceSets([]string{"gpu"}),
        shai.WithTemplateVars(map[string]string{"BRANCH": "main"}),
    )
    if err != nil {
        return err
    }
    cfg.PostSetupExec = &shai.SandboxExec{
        Command: []string{"python", "-m", "agent"},
        Workdir: "/src/agents",
        UseTTY: false,
    }
    sandbox, err := shai.NewSandbox(cfg)
    if err != nil {
        return err
    }
    defer sandbox.Close()
    return sandbox.Run(ctx)
}
```
Key types:
- `SandboxConfig` – Describes the workspace, config path, read/write overlays, selected resource sets, template variables, optional exec command, log writers, verbosity, graceful stop timeout, and image overrides.
- `SandboxExec` – Encapsulates the post-setup command (`Command`, env map, `Workdir`, `UseTTY`).
- `Sandbox` – Interface with `Run`, `Start`, and `Close`. `Start` returns a `SandboxSession` with `ContainerID`, `Wait`, `Stop`, and `Close` helpers for supervising long-running jobs.

Use the Go API when you need to orchestrate multiple sandboxes, integrate with supervisors, or reuse Shai as the execution backend inside unit/integration tests.
