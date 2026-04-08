# TAU Container

Agent container kit for the [TAU](https://github.com/tailored-agentic-units) platform. Provides portable, local-first execution environments where agents operate as users of containerized machines — leveraging local compute for tool execution while connecting to model providers and external services as needed.

## What It Provides

- **OCI-aligned runtime interface** with Docker as the first implementation and containerd/Podman as natural extensions
- **Dual execution models**: toolkit mode (agent on host, container as sandbox) and embedded mode (kernel + agent running inside the container)
- **Image capability manifest** (`/etc/tau/manifest.json`) for declaring available tools, CLIs, packages, and services so agents understand their environment
- **Tool-wrapped persistent shell** and structured file/process tools, all flowing through the standard tool-call paradigm
- **Local-first compute** — container orchestration, tool execution, and file operations run on local hardware; only model inference and external services incur cost

## Where It Fits

```
protocol ─── format ─── agent ─── kernel
               │                    │
               └── container ───────┘
```

The container package sits between the format and kernel layers. It depends on `protocol` and `format` for types and tool definitions. The kernel optionally depends on it for containerized execution. No Go module dependency on `agent` or `kernel`.

## Status

Under development. See [`_project/README.md`](_project/README.md) for the full concept, architecture, and phase roadmap.
