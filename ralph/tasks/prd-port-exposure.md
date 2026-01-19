# PRD: Port Exposure for Container Servers

## Introduction

Enable users to expose TCP and UDP ports from the sandbox container to the host machine so they can run development servers (web servers, APIs, etc.) inside the container and access them locally. Ports are defined in `.shai/config.yaml` as part of resource definitions, with fixed host-to-container port mappings and clear startup output showing which ports are available.

## Goals

- Allow defining fixed port mappings in `.shai/config.yaml` (e.g., `localhost:8000` → container:8000)
- Support both TCP and UDP protocols for flexibility
- Expose ports automatically when resources are loaded
- Print accessible port information during sandbox bootstrap
- Integrate seamlessly with existing resource system (alongside mounts, calls, http)

## User Stories

### US-001: Add ports field to config schema
**Description:** As a developer, I need to define port mappings in `.shai/config.yaml` so they are included when I launch a sandbox.

**Acceptance Criteria:**
- [ ] Update config schema to include `ports` field in resource definitions
- [ ] Schema supports format: `port: 8000` or `port: {host: 8000, container: 8000, protocol: "tcp"}`
- [ ] Validate port numbers are integers 1-65535
- [ ] Validate protocol is `tcp` or `udp`
- [ ] Default protocol to `tcp` if not specified
- [ ] Allow multiple ports in a single resource
- [ ] Typecheck passes

### US-002: Parse and validate port configurations
**Description:** As a developer, I want configuration errors caught at load time so I know immediately if my port setup is invalid.

**Acceptance Criteria:**
- [ ] Config loader validates all port definitions during YAML parsing
- [ ] Error messages identify invalid port numbers or protocols
- [ ] Error messages identify port conflicts (same host port used twice)
- [ ] Validation allows container port to differ from host port
- [ ] Typecheck passes

### US-003: Apply port mappings to Docker container
**Description:** As a developer, I want the container to have access to the ports I defined so my server can bind to them.

**Acceptance Criteria:**
- [ ] Docker container is started with `--publish` flags for each port
- [ ] Handles both TCP and UDP port mappings correctly
- [ ] Maps host port to container port as specified in config
- [ ] Container can bind to the container port and be accessible on host port
- [ ] Typecheck passes

### US-004: Display port information during bootstrap
**Description:** As a developer, I want to see which ports are available when the sandbox starts so I know how to access my server.

**Acceptance Criteria:**
- [ ] Print section showing exposed ports during bootstrap (e.g., "Exposed Ports")
- [ ] Format: `localhost:8000 (tcp) → container:8000`
- [ ] Include both TCP and UDP ports
- [ ] Print occurs after container is ready but before user shell/command runs
- [ ] Print is visible in both verbose and non-verbose modes
- [ ] Typecheck passes

### US-005: Support port configuration via resource sets
**Description:** As a developer, I want to organize ports with other resources (mounts, calls, etc.) so related configuration stays grouped.

**Acceptance Criteria:**
- [ ] Ports defined in a resource are loaded when that resource is activated
- [ ] Multiple resources can define ports (combined when multiple resources active)
- [ ] Ports from apply rules are included automatically
- [ ] CLI `--resource-set` flag correctly activates ports
- [ ] Typecheck passes

### US-006: Document port configuration in examples
**Description:** As a developer, I want example configurations showing how to set up ports so I can copy and adapt them.

**Acceptance Criteria:**
- [ ] Add port configuration example to `.shai/config.yaml` documentation
- [ ] Show basic HTTP server example (port 8000)
- [ ] Show multiple ports example (web + API)
- [ ] Show UDP example if applicable
- [ ] Include explanation of host vs container port mapping

## Functional Requirements

- FR-1: Add `ports` field to resource definitions in config schema
- FR-2: Support port syntax: `port: 8000` (assumes host=8000, container=8000, tcp) or explicit object format
- FR-3: Support protocol specification: `protocol: "tcp"` or `protocol: "udp"`
- FR-4: Validate port numbers are in valid range (1-65535)
- FR-5: Detect and report port conflicts at config load time
- FR-6: Pass Docker `--publish host_port:container_port/protocol` flags during container creation
- FR-7: Collect all active ports from selected resource sets
- FR-8: Print exposed ports section during bootstrap showing all active ports with protocol and mapping
- FR-9: Port output format: `localhost:HOST_PORT (PROTOCOL) → container:CONTAINER_PORT`
- FR-10: Ports print after "Container created" message but before shell/command execution

## Non-Goals

- No dynamic port allocation (ports are fixed and explicit)
- No port range support (each port specified individually)
- No automatic port scanning or availability checking
- No port-based reverse proxy or routing
- No UDP multicast or broadcast support beyond basic UDP binding
- No integration with health checks or liveness probes

## Design Considerations

- Align port configuration format with existing mount/call/http definitions
- Keep port display minimal but informative during startup
- Avoid conflicting with ports already in use on host (user responsibility, but validate config-level conflicts)
- Reuse existing config loader validation patterns

## Technical Considerations

- Update `Config` struct in `internal/shai/runtime/config/` to include Port type
- Update config loader validation to check ports during `LoadConfig()`
- Modify `EphemeralRunner` to pass ports to Docker client as `--publish` flags
- Update bootstrap script output logic to print exposed ports
- Port validation should mirror Docker's validation (1-65535)
- Container port bindings must be specified even if same as host port

## Success Metrics

- Users can expose a web server in under 5 lines of config
- All exposed ports visible in startup output
- No configuration errors make it to Docker layer (caught by config validator)
- Can run Flask/Node/Go dev servers accessible from host browser

## Open Questions

- Should we support container IP specification (e.g., `127.0.0.1:8000` vs `0.0.0.0:8000`)? Or always bind to all interfaces?
- Should port conflicts be a hard error or warning if the same port is used in different resource sets?
- Should we support port ranges in future (e.g., `8000-8010`)? (Out of scope for MVP)
