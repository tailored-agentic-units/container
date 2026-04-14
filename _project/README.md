# TAU Container

## Vision

Provide a standardized agent container kit for the TAU platform — enabling agents to operate as users of containerized machines that leverage local compute while integrating with hosted services. By normalizing locally run containers as the primary execution model, agent workloads consume free local hardware for compute-intensive tasks and connect to external services (including AI model providers, whether cloud-hosted or local) only when needed. The same container runs identically on a developer laptop, home server, or cloud infrastructure — making deployment location a configuration decision, not an architectural one.

## Core Premise

Cloud compute is the most expensive component of AI-integrated infrastructure. The TAU Container library inverts the default assumption that agent workloads must run in the cloud by establishing containers as portable, local-first execution environments. An agent running inside a container has access to the same tools and files that a human user would — shell sessions, installed CLIs, package managers, and workspace files — while connecting outward to model providers and any external services the workflow requires.

This model delivers three compounding benefits:

1. **Cost efficiency**: Local compute is effectively free. Only services that provide genuine value (AI model inference, managed databases, object storage) incur cost. Container orchestration, tool execution, and file operations all happen on local hardware.
2. **Development velocity**: The same container that runs in production runs on a developer's machine. Kernel features, agent behaviors, and tool integrations are prototyped and validated locally before any remote deployment.
3. **Deployment portability**: A container is a container. The same image runs on a laptop for development, a home server for persistent agents, or cloud infrastructure when scale demands it. The agent's integration with external services is identical regardless of where the container is hosted.

## Phases

| Phase | Focus Area | Version Target |
|-------|-----------|----------------|
| Phase 1 - Runtime Foundation | OCI-aligned runtime interface, Docker implementation, container lifecycle, one-shot exec, file copy, image capability manifest | v0.1.0 |
| Phase 2 - Agent Tool Bridge | Tool-wrapped persistent shell, structured file/process tools, dynamic tool generation from manifest, agent integration surface | v0.2.0 |
| Phase 3 - Image Management | Image builder from configuration, pre-built image profiles, resource limits, health checks, networking for service integration | v0.3.0 |

## Architecture

### Dependency Position

```
protocol (L0) ─── format (L1) ─── agent (L2) ─── kernel (L3)
                     │                               │
                     └──── container (L2.5) ─────────┘
```

The container package depends on `protocol` (types) and `format` (tool definitions). The kernel optionally depends on the container package for toolkit mode integration. The container package has no Go module dependency on `agent` or `kernel`.

### Execution Models

The container package supports two execution models through the same `Runtime` interface and manifest convention. The difference is what lives inside the image and how the host interacts with it.

#### Model A: Toolkit Mode

The agent runs on the host. The container is a headless execution environment — a sandbox of tools and files. The agent interacts with the container through tool calls backed by Docker exec and file copy operations.

```
┌──────────────────────┐      ┌──────────────────────────┐
│  Host                │      │  Container               │
│                      │      │                          │
│  Agent ──── Kernel   │─────►│  Shell, Files, CLIs      │
│    │                 │ exec │  (local compute)         │
│    │                 │ copy │                          │
│    └─────────────────│──────│──────────────────────────│──►  Model Provider
│                      │      │                          │
└──────────────────────┘      └──────────────────────────┘
```

**Best for**: Rapid prototyping, connecting an existing `tau/agent` instance to a sandboxed environment, lightweight experimentation without embedding a full kernel.

#### Model B: Embedded Mode

The container image includes a pre-built kernel binary and agent configuration. The kernel boots inside the container, connects outward to the model provider, and uses tools natively. The host communicates with the container-agent via HTTP API.

```
┌──────────────────────┐      ┌──────────────────────────┐
│  Host                │      │  Container               │
│                      │      │                          │
│  Container Manager   │─────►│  Kernel ──── Agent       │
│  (lifecycle, API)    │ HTTP │    │                     │
│                      │      │    ├── Shell, Files, CLIs│
│                      │      │    │   (native execution)│
│                      │      │    │                     │──►  Model Provider
│                      │      │    └── External Services │──►  (per manifest)
│                      │      │                          │
└──────────────────────┘      └──────────────────────────┘
```

**Best for**: Production workloads, persistent agents, self-contained agentic units that leverage local compute and connect to external services. The kernel binary is a build artifact baked into the image — not a Go dependency of the container package.

**Dependency note**: Model B does not create a Go module dependency from `container` to `kernel`. The kernel binary is included in the container image at Docker build time (via Dockerfile `COPY`). The container package manages the image and lifecycle at the OCI level — it doesn't import kernel code.

### Package Structure

```
container/
├── container.go        # Container type, lifecycle states, CreateOptions
├── runtime.go          # Runtime interface (OCI-aligned)
├── registry.go         # Factory type, package-level Register/Create/ListRuntimes
├── manifest.go         # Image capability manifest types and parsing
├── tools.go            # Tool definition generators (format.ToolDefinition)
├── shell.go            # Persistent shell session type
├── docker/             # Sub-module: Docker Engine API implementation
│   ├── go.mod
│   ├── docker.go       # Runtime interface implementation
│   └── register.go     # Explicit Register() function
├── _project/
│   └── README.md       # This file
├── go.mod
└── README.md
```

### Runtime Interface

OCI-aligned abstraction over container runtimes. The root module defines the interface with zero heavy dependencies. Implementations live in sub-modules with their own `go.mod`.

```go
type Runtime interface {
    Create(ctx context.Context, opts CreateOptions) (*Container, error)
    Start(ctx context.Context, id string) error
    Stop(ctx context.Context, id string, timeout time.Duration) error
    Remove(ctx context.Context, id string, force bool) error
    Exec(ctx context.Context, id string, opts ExecOptions) (*ExecResult, error)
    CopyTo(ctx context.Context, id string, dst string, content io.Reader) error
    CopyFrom(ctx context.Context, id string, src string) (io.ReadCloser, error)
    Inspect(ctx context.Context, id string) (*ContainerInfo, error)
}
```

The first implementation targets the Docker Engine API via `container/docker`. The interface design is containerd-compatible, allowing a native containerd implementation as a future sub-module — which would cover Podman and other containerd-backed runtimes.

### Local-First Execution Model

The container runs on local hardware. The agent (whether on the host in toolkit mode or inside the container in embedded mode) connects outward to services as needed:

- Tool execution, file operations, build/test cycles — **local compute** (free)
- AI model inference — **model provider** (cloud-hosted or local, e.g., Ollama)
- Managed services — **declared in manifest** (storage, databases, APIs)

This is where cost savings compound. The compute-heavy agentic loop (tool calls, file parsing, compilation, test suites) runs on hardware the user already owns. Only model inference and explicit external service calls leave the local machine.

### Image Capability Manifest

Container images declare their capabilities via a well-known file at `/etc/tau/manifest.json`. This allows agents to understand what tools, CLIs, packages, and services are available in their environment before they begin working.

```json
{
  "version": "1",
  "name": "tau-go-dev",
  "description": "Go development environment with common tools",
  "base": "alpine:3.21",
  "shell": "/bin/bash",
  "workspace": "/workspace",
  "env": {
    "GOPATH": "/go",
    "PATH": "/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/bin:/bin"
  },
  "tools": {
    "go": { "version": "1.26", "description": "Go compiler and toolchain" },
    "git": { "version": "2.47", "description": "Version control" },
    "gh": { "version": "2.65", "description": "GitHub CLI for repository operations" },
    "mise": { "version": "2025.1", "description": "Runtime version manager" },
    "fzf": { "description": "Fuzzy finder for interactive selection" },
    "eza": { "description": "Modern ls replacement with git integration" }
  },
  "services": {
    "azure-blob": { "description": "Azure Blob Storage for artifact persistence" },
    "postgres": { "description": "PostgreSQL database for structured data" }
  },
  "options": {
    "docker": { "healthcheck": "/bin/sh -c 'exit 0'" }
  }
}
```

The schema is strict: `container.Parse` rejects any top-level field not defined by the `Manifest` type. Runtime- or image-specific configuration that tau itself does not interpret belongs under `options`, the single sanctioned pass-through slot (mirrors the `Options map[string]any` convention at `tau/protocol/config` and `tau/format`). Anything else — a misspelled field, a field from an older schema version, or a field added by a non-tau tool — surfaces as a decode error so drift is caught at load time rather than silently ignored.

The manifest is parsed at container start and used to:
- Inform the agent's system prompt about available capabilities
- Generate context-aware tool descriptions
- Declare available external services and their purpose
- Validate that expected tools are present

Images without a manifest fall back to a base set of assumptions (POSIX shell, standard coreutils). `container.Fallback()` returns a non-nil `*Manifest` that passes `Validate` for this case.

### Agent Interaction Model

All container interactions flow through the tool-call paradigm, aligning with the kernel's agentic loop. Two categories of tools:

**Built-in tools** (always available):
- `shell` — persistent session with maintained cwd, env vars, and shell state across invocations
- `file_read`, `file_write`, `file_list` — structured file operations with clean JSON responses
- `process_list`, `process_kill` — process management

**Manifest-declared tools** (dynamically generated from the image manifest):
- Each entry in the manifest's `tools` section generates context for the agent's system prompt
- The agent knows it can invoke `gh`, `go`, `mise`, etc. through the persistent shell tool
- Tool descriptions include version information and capability summaries

The persistent shell is modeled as a dedicated `Shell` type that wraps `Runtime.Exec` with a long-running PTY-attached session, maintaining state across tool invocations.

### Sub-module Convention

Following established TAU patterns:
- Root module exposes interfaces and types with zero heavy dependencies
- `container/docker` is a sub-module with its own `go.mod` importing the Docker client SDK
- Explicit `Register()` function — no `init()` auto-registration
- Per-module CHANGELOG.md and README.md

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Package name | `container` | Directly describes what's provided; fits TAU single-word convention |
| Execution model | Dual-mode (toolkit + embedded) | Toolkit mode for prototyping with `tau/agent`, embedded mode for production with full kernel inside. Same Runtime interface, different image contents. |
| Local-first compute | Containers run on local hardware by default | Compute is expensive; local hardware is free. Only model inference and external services incur cost. |
| Runtime abstraction | OCI-aligned interface | Clean extension point; Docker first via sub-module, containerd/Podman follow naturally |
| Interaction model | Tool-wrapped persistent shell + structured tools | Everything flows through the standard tool-call paradigm; persistent shell maintains state; structured tools provide clean JSON |
| Capability discovery | Image manifest at `/etc/tau/manifest.json` | Agents must know what tools and services are available; well-known path is simple, versionable, and discoverable |
| Manifest in Phase 1 | Include early | Trivial JSON struct with zero runtime dependencies; enables Phase 2 tool bridge to focus on shell and integration |
| No agent/kernel Go dependency | Root module depends only on protocol + format | Container stays at L2.5. Kernel binary in embedded mode is a build artifact, not a Go import. |

## Dependencies

### Root Module (`github.com/tailored-agentic-units/container`)
- `github.com/tailored-agentic-units/protocol` — foundational types
- `github.com/tailored-agentic-units/format` — tool definitions (`format.ToolDefinition`)

### Docker Sub-module (`github.com/tailored-agentic-units/container/docker`)
- `github.com/tailored-agentic-units/container` — root interface
- `github.com/docker/docker/client` — Docker Engine API

## Integration Points

- **Kernel**: In toolkit mode, kernel consumes container tool definitions and wires them into the agentic loop. In embedded mode, the kernel runs inside the container natively.
- **Agent**: In toolkit mode, agent calls container tools through the standard `agent.Tools()` path. In embedded mode, agent runs inside the container and uses tools directly.
- **Orchestrate**: Multiple containerized agents can be coordinated through the existing Hub/Participant patterns — each container is a Participant.
- **Format**: Container tools are defined as `format.ToolDefinition`, ensuring compatibility with all wire formats (OpenAI, Converse).
- **Provider**: Model providers connect agents to AI models — cloud-hosted (Azure, Bedrock) or local (Ollama). Provider configuration is independent of container configuration. The container provides the execution environment; the provider provides model connectivity.
- **External Services**: Availability of additional services (storage, databases, APIs) is declared in the image capability manifest. The manifest informs the agent what services it can interact with and their purpose.

## Open Questions

- Should the manifest support declaring custom tool definitions (full JSON Schema parameters) in addition to CLI tool descriptions?
- What is the right mechanism for volume mounts and host-container file sharing beyond `CopyTo`/`CopyFrom`?
- Should the container package provide its own base images, or rely entirely on user-provided images with the manifest convention?
- How should networking between multiple containers be managed for multi-agent orchestration scenarios?
- What is the optimal pattern for credential passthrough — should cloud service credentials be injected at container creation time, or should the container inherit host credentials via mounted paths?
