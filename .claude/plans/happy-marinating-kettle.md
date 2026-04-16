# Plan — Issue #14: docker-hello runnable example

## Context

Issue [#14](https://github.com/tailored-agentic-units/container/issues/14) is the final sub-issue of [Objective #3 — Docker Runtime Implementation](https://github.com/tailored-agentic-units/container/issues/3). Sub-issues #11/#12/#13 implemented the `Runtime` interface (lifecycle, exec/copy, inspect+manifest), and the Phase 1 release session has tagged `v0.1.0` (root) and `docker/v0.1.0` (Docker sub-module). What is missing: a downstream consumer that exercises the *published* contract from a separate module — proof that an external caller can pull tagged releases and drive the runtime end-to-end.

The work lives in `~/tau/examples` (`github.com/tailored-agentic-units/examples`), not in the `container` repo, because the examples module is the cross-repo integration point that consumes tagged releases of every TAU library. Issue #14 is tracked on the container repo only so the Obj #3 sub-issue roll-up stays honest.

The implementation is intentionally smoke-level — no assertions, no test framework. The example is a literate demonstration of the published API surface, including the explicit nil-manifest case so readers see the documented `container.Fallback()` substitution pattern.

## Layout decision: nest under `container/`, not `cmd/`

The container library will accumulate further examples through Phase 2–4 (persistent shell wrapper, tools introspection, image-builder demos, manifest-carrying images, label-based listing, alternate runtimes). Adding `cmd/docker-hello/` now and grafting on more siblings later would crowd the `cmd/` directory and obscure that those examples belong to one library.

The repo already establishes the right pattern for this case: `orchestrate/` contains seven sibling demos (`phase-01-hubs/`, `phase-02-03-state-graphs/`, …, `darpa-procurement/`) plus an index `orchestrate/README.md` that lists them. Each demo is `go run ./orchestrate/<name>/`. Container should follow the same shape:

```
~/tau/examples/
├── cmd/prompt-agent/        # one-off CLI tool — stays in cmd/
├── orchestrate/             # orchestrate library demos
│   ├── README.md
│   ├── phase-01-hubs/
│   └── …
└── container/               # container library demos (NEW)
    ├── README.md
    └── docker-hello/        # this issue
        ├── main.go
        └── README.md
```

`cmd/prompt-agent/` stays where it is — it is a multi-protocol CLI utility, not a library demo.

## Cross-repo session note

- Branch `14-docker-hello` is already created on the **examples** repo (`~/tau/examples`).
- This plan file lives under `~/tau/container/.claude/plans/` because the dev-workflow session is anchored to the container repo (the issue's home), but every code change in the implementation phase happens in `~/tau/examples`.
- The Phase 3 implementation guide (workflow convention `.claude/context/guides/[issue]-[slug].md`) will be written to `~/tau/examples/.claude/context/guides/14-docker-hello.md`. This will be the first file under `.claude/context/` in the examples repo — the directory chain must be created (`mkdir -p`).

## Files to create

### 1. `~/tau/examples/container/README.md`

Index for container library examples. Mirrors `orchestrate/README.md` shape:

- One-paragraph header describing what the directory holds (TAU container library demos exercising the published `tau/container` and runtime sub-module contracts).
- **Prerequisites** — Docker daemon reachable; `docker pull alpine:3.21` for any demo that uses alpine.
- **Examples** table — single row today (`docker-hello | Docker runtime end-to-end (lifecycle, exec, manifest fallback) | go run ./container/docker-hello/`). Future examples land as additional rows.
- **References** — cross-link to [`tailored-agentic-units/container`](https://github.com/tailored-agentic-units/container) library docs.

### 2. `~/tau/examples/container/docker-hello/main.go`

Single-package, single-`main` program. Imports order matches `cmd/prompt-agent/main.go` and `orchestrate/phase-01-hubs/main.go`: stdlib group, then TAU packages (`container`, `container/docker`).

Flow (mirrors the Approach section of the issue, with two deliberate refinements):

1. `docker.Register()` then `rt, err := container.Create("docker")`.
2. `rt.Create(ctx, container.CreateOptions{Image: "alpine:3.21", Name: "tau-docker-hello", Labels: map[string]string{"example": "docker-hello"}})` — explicit `Name` makes the Exec output deterministic and keeps the example's printed greeting stable across runs (the issue's snippet echoes `c.Name`, which would otherwise be empty since `dockerRuntime.Create` returns `Name: opts.Name`).
3. `rt.Start(ctx, c.ID)`.
4. `rt.Exec(ctx, c.ID, container.ExecOptions{Cmd: []string{"echo", "hello from", c.Name}, AttachStdout: true})` — print `string(res.Stdout)`.
5. `rt.Inspect(ctx, c.ID)` — print `info.Manifest` (expected `<nil>` because alpine carries no `/etc/tau/manifest.json`), then substitute `m := container.Fallback()` and print `m.Shell` (`/bin/sh`) to demonstrate the documented substitution pattern. Satisfies acceptance criterion #6 ("Example demonstrates the nil-manifest case explicitly so readers see the `Fallback()` substitution pattern").
6. `rt.Stop(ctx, c.ID, 5*time.Second)` then `rt.Remove(ctx, c.ID, false)`.

Error policy: `log.Fatalf("step: %v", err)` at each call site — matches the prompt-agent convention (no `panic`). The issue notes "failure surfaces as a panic on unwrapped error" but `log.Fatalf` is more idiomatic in this repo and equivalent in user-facing behavior (process exits non-zero with a diagnostic line).

Context: a single `context.Background()` for the whole flow is sufficient for a smoke demo. No signal handling — this is a literate example, not a production CLI.

### 3. `~/tau/examples/container/docker-hello/README.md`

Sections (matches the prompt-agent README rhythm):

- **Prerequisites** — Docker daemon running and reachable (`docker info`); pull base image with `docker pull alpine:3.21` (the example does not pull on its own).
- **Run** — `go run ./container/docker-hello/` from the repo root.
- **Expected output** — three lines, exactly:
  - `hello from tau-docker-hello`
  - `manifest: <nil> (alpine carries no tau manifest)`
  - `fallback shell: /bin/sh`
- **What this demonstrates** — explicit pointer at the nil-manifest path and the `container.Fallback()` substitution.
- **References** — cross-link to the [`tailored-agentic-units/container`](https://github.com/tailored-agentic-units/container) repo for library docs.

## Files to modify

### 4. `~/tau/examples/README.md`

Three insertions (mirroring how `orchestrate/` appears today):

- **Structure block**: add `container/           -- Container library examples` directly after the `orchestrate/` line, preserving column alignment.
- **Sub-READMEs list**: add `- [container examples](./container/README.md) -- Docker runtime lifecycle, exec, and manifest-fallback patterns` after the orchestrate-examples bullet (single bullet covers all container demos current and future, matching the orchestrate treatment).
- **Dependencies table**: append two rows after the orchestrate row:
  - `| `github.com/tailored-agentic-units/container` | OCI-aligned container runtime abstraction |`
  - `| `github.com/tailored-agentic-units/container/docker` | Docker Engine API runtime implementation |`

### 5. `~/tau/examples/go.mod` and `go.sum`

Run from `~/tau/examples`:

```
go get github.com/tailored-agentic-units/container@v0.1.0
go get github.com/tailored-agentic-units/container/docker@docker/v0.1.0
go mod tidy
```

Both releases were verified present on the container repo. No `replace` directives. The Docker SDK pulls in via the docker sub-module's transitive deps — `go mod tidy` will materialize these.

## Critical files (read-only references)

- `~/tau/container/runtime.go` — `Runtime` interface contract and per-method cancellation semantics.
- `~/tau/container/container.go` — `CreateOptions`, `ExecOptions`, `ContainerInfo`, `State` constants.
- `~/tau/container/manifest.go` — `Manifest`, `Fallback()`, `ManifestPath`, `ManifestVersion`.
- `~/tau/container/docker/docker.go` — `Register()`, `LabelManaged`, `LabelManifestVersion`.
- `~/tau/examples/orchestrate/README.md` — index README shape to mirror for `container/README.md`.
- `~/tau/examples/orchestrate/phase-01-hubs/main.go` — sub-directory demo conventions (imports, ctx, log usage).
- `~/tau/examples/cmd/prompt-agent/main.go` — error-handling and import-order convention to mirror.

## Verification

Run from `~/tau/examples` after Phase 4 implementation:

1. `go build ./container/docker-hello/...` — must succeed (acceptance criterion #5, adapted to new path).
2. `go vet ./container/docker-hello/...` — clean.
3. With Docker daemon running and `alpine:3.21` pulled: `go run ./container/docker-hello/` — must print the three expected lines, exit zero.
4. `docker ps -a --filter label=tau.managed=true` immediately after the run — must show no `tau-docker-hello` container (Remove succeeded).

## Workflow handoff

Per task workflow Phase 3, after exiting plan mode I will:

1. `mkdir -p ~/tau/examples/.claude/context/guides`.
2. Write the implementation guide to `~/tau/examples/.claude/context/guides/14-docker-hello.md` containing literal source for `container/README.md`, `container/docker-hello/main.go`, `container/docker-hello/README.md`, and exact diff hunks for `examples/README.md` (no godoc, no tests, no PM updates per Phase 3 conventions).
3. **Stop** for developer execution (Phase 4). Phases 5–8 (testing, validation, documentation, closeout) follow on developer signal.
