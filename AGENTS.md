# Agent Instructions for Gorums

Gorums is a Go framework for fault-tolerant systems built on quorum calls.
The `protoc-gen-gorums` plugin generates the Gorums API from `.proto` files.
Read `doc/dev-guide.md` before architectural changes and `doc/user-guide.md` before API changes.

## Modules

Three modules joined by `go.work`: `github.com/relab/gorums` (root), `benchkit`, and `examples`.
benchkit imports gorums, never the reverse.
Keep the root `go.mod` free of benchmarking and orchestration dependencies.

## Working With the Maintainer

- STOP and ASK when a design decision is unclear.
  Discuss architectural changes and backward-incompatible changes before implementing them.
- Prepare a plan before larger features and refactors.
- Push back on bad ideas with technical reasoning.
- Private issues, designs, reviews, plans, and notes live in `.scratch/<feature>/`; never stage them.
  A separate issue found along the way becomes a numbered file in `.scratch/<feature>/issues/`, not scope creep.

## Explaining to the Maintainer

The maintainer's job is oversight, so make output easy to understand.

- Write prose in plain, controlled English, about 80% of the way to ASD-STE100: short sentences, one idea each, active voice, simple words.
- For a structure, flow, or message exchange, draw a diagram instead of describing it.
- For a large design or review, write a discardable HTML page to `.scratch/<feature>/`.

## Code Generation

Generated `zorums_*_gorums.pb.go` files in `cmd/protoc-gen-gorums/dev/` are outputs; edit their sources, then run `make dev`.

- Template change: edit `cmd/protoc-gen-gorums/gengorums/template_*.go`.
- Static code change: edit the non-`zorums_*` files in `cmd/protoc-gen-gorums/dev/`.
- `make genproto` regenerates every `_gorums.pb.go` file.

Never edit any generated file to satisfy a linter; fix the generator or its inputs and regenerate.

## Tests

Follow TDD: write a failing test, watch it fail, make it pass, refactor.
All tests pass before work is complete; fix failing tests rather than deleting or skipping them.
Use only Go's `testing` package.
The default mode uses in-memory bufconn; `-tags=integration` uses real TCP.

- Use table-driven tests for repeated logic and subtests for related cases.
- Name tests `TestFileNameFeature`, e.g., `TestQuorumCallFeature` in `quorumcall_test.go`.
- Use the `gorumstest` package for all setup: `Config`, `Node`, `Servers`, `LocalServers`, `UnreachableConfig`, `Context`, `WaitUntil`, `DialOptions`, `WithStopFunc`, `WithPreConnect`.
  It owns listeners, cleanup order, and goroutine-leak checks, which keeps `-count=N` runs race-free.
- If `gorumstest` falls short, add a focused, documented helper there.
- Never bind, release, and re-bind a port by hand (`net.Listen("tcp", ":0")`); it races other binders.
  If a test needs behavior `gorumstest` cannot express, ask the maintainer first.

## Code Style

- Match the surrounding code; preserve comments unless they are wrong.
- Use current Go and the standard library (`slices`, `maps`, `rand/v2`, iterators, generics).
- Name by type, consistently: the same name for the same type everywhere (`nodes NodeSource`, `callCtx *CallContext[...]`).
  Reserve `opt`/`opts` for functional options.
- Fold overlapping concepts into one name or abstraction when their contracts align.
- Never add an exported function whose only body calls an unexported twin.
  Export the original and update its call sites.
  Exceptions: wrappers that add behavior, and re-exports of internal generics that a type alias cannot express (`runtime/gorumsimpl` constructors, `MapRequest`, `MapResponse`, `NewConfig`, `WithNodeList`).
- Before finishing a session that changes Go code: run `make modernize` and review its changes, then run `make goplscheck` and fix every diagnostic in non-generated code (including hints), and rerun until clean.

## Documentation

- Every exported identifier has a succinct doc comment; every package has one (typically `doc.go`).
- Comments state the current contract and constraints, positively.
  History, algorithm steps, and rejected designs belong in tests, commit messages, or `.scratch/`.
- Link other declarations with `[Identifier]`, `[Type.Method]`, or `[pkg.Identifier]`.
- Go source never references repository Markdown paths; state the constraint inline.
- Update `doc/` whenever public APIs or behavior change.
- Markdown: one sentence per line.

## Git

- Branch from `master` as `feature/[ID/]short-description` or `fix/[ID/]short-description`.
- One coherent change per commit, with its tests and docs; never mix unrelated code, docs, or formatting.
- Commit generated files last, in their own commit, after the templates and static code.
- Check `git status` before staging; never `git add -A` blindly.
- Subject format: `package-name: descriptive subject`, at most 75 characters, no Markdown.
  Example: `gorums: add quorum call timeout option`.
- Never add a `Co-Authored-By` trailer naming an AI agent.
- At the end of a session, give a proposed message for each commit in its own plain-text fenced block.
