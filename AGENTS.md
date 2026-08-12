# Agent Instructions for Gorums

Gorums is a framework for building fault-tolerant distributed systems using quorum-based abstractions.
This document provides context and rules for AI coding assistants.

## Project Overview

Gorums provides:

- Flexible quorum call abstractions for distributed systems
- Code generation via `protoc-gen-gorums` compiler plugin
- gRPC-based RPC communication
- Supported communication styles: unicast, multicast, quorumcall, async and correctable quorum calls

**Key Technologies:**

- Language: Go 1.25+
- Build: Make
- Protocol: Protocol Buffers (protobuf)
- Testing: Go testing framework
- Code Generation: Custom protoc plugin

## Repository Structure

```text
gorums/
├── cmd/protoc-gen-gorums/     # Compiler plugin for code generation
│   ├── dev/                   # Static code + generated code examples
│   └── gengorums/             # Compiler logic + templates
├── benchkit/                  # Separate module: measurement and benchmarking
│   ├── proto/                 # .proto sources for the benchkit module
├── examples/                  # Separate module: example implementations
├── internal/                  # Internal packages
├── doc/                       # Documentation
└── *.go                       # Core library files
```

The repository holds three modules: `github.com/relab/gorums` at the root,
`github.com/relab/gorums/benchkit`, and `github.com/relab/gorums/examples`.
They are joined by `go.work`.
The dependency edge runs one way: benchkit imports gorums, never the reverse.
Keep the root `go.mod` free of benchmarking and orchestration dependencies.

## Development Rules

### General Guidelines

- For larger features and refactors, prepare a plan before coding
- STOP and ASK if unsure about design decisions
- ALWAYS write tests for new features and bug fixes
- Large changes must be broken into small, manageable units to be committed separately
- Use scoped commits: each commit should contain one coherent change, including any tests and documentation required by that change.
- NEVER mix unrelated code, documentation, or formatting changes in the same commit.
- Issues, designs, reviews, plans, specifications, and task notes are private local artifacts.
  Store them under `.scratch/<feature>/`; never stage or commit them.
- If you discover a separate issue while working, add or update its numbered file under `.scratch/<feature>/issues/` instead of expanding the current change.
- At the end of a session, provide a proposed commit message for each scoped commit in a separate plain-text fenced block that can be copied verbatim.
- Commit messages must use a clear, human-readable subject of at most 75 characters and must not contain Markdown links or formatting.
- Scoped commit prefixes are required: package-name: descriptive subject. For example: `gorums: add quorum call timeout option`.
- NEVER add a `Co-Authored-By` trailer naming an AI agent to a commit message.

### Code Generation Workflow

**NEVER directly edit files prefixed with `zorums_*_gorums.pb.go` in `cmd/protoc-gen-gorums/dev/`**

These files are generated from templates. Instead:

1. **For Template Changes:**
   - Edit template in `cmd/protoc-gen-gorums/gengorums/template_*.go`
   - Run `make dev` to regenerate `zorums_*_gorums.pb.go` files

2. **For Static Code Changes:**
   - Edit files in `cmd/protoc-gen-gorums/dev/` that are NOT prefixed with `zorums_*`
   - Run `make dev` to bundle changes into `template_static.go`

3. **After Any Template or Static Code Changes:**
   - Run `make dev` to regenerate `zorums_*_gorums.pb.go` files
   - Or run `make genproto` to regenerate all _gorums.pb.go files

### Testing Requirements

- Use the `github.com/relab/gorums/gorumstest` package for common test setup.
- Use `gorumstest.Config` for a multi-server configuration, `gorumstest.Node` for a single server node, `gorumstest.Servers` for server addresses, and `gorumstest.LocalServers` for symmetric in-process server groups.
- Use `gorumstest.NoDialedConfig` when a test needs configuration construction without dialing servers.
- Use `gorumstest.Context` for test-scoped timeouts and `gorumstest.WaitUntil` for bounded polling.
- Use `gorumstest.DialOptions` or `gorumstest.InsecureDialOptions` for test connections, and use `gorumstest.WithStopFunc` or `gorumstest.WithPreConnect` when a failure-path test needs server lifecycle control.
- These helpers own listener allocation, cleanup ordering, and goroutine-leak checks. They keep listeners open for their intended lifetime, which makes repeated runs with `go test -count=N` race-free.
- If the provided `gorumstest` helpers are insufficient, add a focused helper to that package and document its contract and usage.
- NEVER hand-roll listener or port allocation in tests (e.g., binding `net.Listen("tcp", ":0")`, reading `.Addr()`, closing the listener, and later re-binding that port).
  The bind-release-rebind pattern races other binders and has caused `-count` flakiness.
  If a test genuinely needs behavior the framework cannot express,
  STOP and ask the maintainer for permission before introducing an alternative approach, and document why in the test.
- Always write table-driven tests when same logic needs to be tested with multiple inputs
- Organize related tests using subtests
- Test names should be capitalized, like TestFileNameFeatureName, e.g., TestQuorumCallFeatureName, for some feature in `quorumcall_test.go`
- Run relevant tests after each change
- NEVER delete failing tests - fix the underlying issue - unless the test is no longer relevant
- NEVER skip tests or ignore failures
- NEVER use another testing framework than Go's testing package
- If addressing a test failure requires significant changes, stop and ask for guidance
- Test coverage should be comprehensive
- ALL tests must pass before considering work complete

### Testing Strategy

Follow Test Driven Development (TDD) when adding features or fixing bugs:

1. Write failing test
2. Confirm test fails
3. Write minimal code to pass test
4. Confirm test passes
5. Refactor if needed

### Code Style and Conventions

- **Match existing code style** - consistency within files is paramount
- **Follow Go conventions** - use `gofmt`, follow effective Go practices
- Before wrapping up a session that changes Go code, run `make modernize`, review its changes, and run `make goplscheck`.
- Resolve every `make goplscheck` diagnostic in non-generated Go source before considering the work complete.
  This check includes hint-level gopls simplifications that `go fix` and the standalone modernize suite do not cover.
  Apply the corresponding gopls quick fix or make the equivalent source edit, then rerun the check.
- Never edit generated files to satisfy modernization diagnostics; update the generator or its inputs and regenerate instead.
- **Use Go's standard library**
  - use up-to-date standard library features when relevant
  - use recent versions of packages: slices, maps, sync, rand/v2
  - use for-range iterators with yield when applicable
  - use generics when appropriate
- **Use meaningful names** - reflect domain concepts, not implementation details
- **Name by type, consistently** - use the same variable/parameter name for a given type across the codebase (e.g. `nodes NodeSource`, `callCtx *CallContext[...]`); do not reuse a generic name like `opt`/`opts` for a value whose type is not itself a functional option
- **Limit concept sprawl** - consolidate overlapping ideas under one general concept, name, or abstraction whenever their contracts align
- **Preserve comments** unless they are demonstrably incorrect
- **No forwarding functions/methods** - never add an exported function or method whose only body is a call to an unexported one of the same shape (e.g. `func Foo() T { return foo() }`) purely to expose it.
  Export the unexported one directly (rename it, update its doc comment) and change call sites to use the exported name.
  The only exception is when the wrapper adds real behavior (validation, combining multiple calls, adapting a signature) beyond exposing the name.

### Add documentation

- Each exported function, type, and method must have a succinct Go doc comment describing its purpose and usage
- Non-exported functions, types, and methods should have comments if their purpose is not immediately clear
- Keep comments clear and readable. Describe the current contract and important constraints; leave out algorithmic steps, change history, and alternative designs to tests, commit messages, or private work artifacts.
- State behavior directly and positively instead of contrasting it with hypothetical behavior.
- Go source files must not reference repository Markdown paths. Replace such references with a short, self-contained explanation of the relevant constraint.
- When a doc comment references another declaration (in the same or a different package),
  use Go's doc comment link syntax with square brackets - `[Identifier]`, `[Type.Method]`, or `[pkg.Identifier]` -
  so it renders as a clickable link (see <https://go.dev/doc/comment#links>); never mention another function or type by bare name in prose
- Each Go package (typically in doc.go) should have a comment block describing its purpose:

  ```go
  // Package gorums provides quorum call abstractions for distributed systems.
  package gorums
  ```

- **Update user/developer documentation** - whenever public APIs or behaviors change, update relevant documentation in `doc/`

### Git Workflow

- Main branch: `master`
- Work on feature branches
- Create new branches for significant changes, unless already working on a feature branch
- Feature branches should be named: `feature/short-description` or `fix/short-description`
- If there is an associated GitHub issue, include its ID in the branch name: `feature/123/short-description`
- Commit individual units of work with clear, descriptive messages
- Never use `git add -A` without first checking `git status`
- Run tests before committing
- DO NOT commit anything ever again unless explicitly asked to.
- DEFINITELY DO NOT push ever, even if asked.

## Building and Testing

### Build Commands

```bash
# Generate `zorums_*_gorums.pb.go` files in `cmd/protoc-gen-gorums/dev/`
make dev

# Generate _gorums.pb.go files across the project
make genproto

# Build everything
make

# Force rebuild
make -B

# Install protoc-gen-gorums plugin
make installgorums

# Install required tools
make tools
```

### Testing Commands

```bash
# Run all tests with verbose output and a timeout (use to avoid hanging tests)
go test -v -timeout=15s ./...

# Ensure tests are actually run (not skipped by cache)
go test ./... -count=1

# Run integration tests directly
go test -tags=integration ./...
```

#### Testing Modes

Gorums has two testing modes:

- **Default (bufconn):** Uses in-memory connections for faster tests during development.
- **Integration:** Uses real TCP connections with `-tags=integration`, for end-to-end validation.

For most development work, use the default bufconn mode.
Use integration mode for performance benchmarking and network-specific validation.

## Working with Protocol Buffers

- Service definitions use `.proto` files
- The `protoc-gen-gorums` plugin extends standard protobuf/gRPC generation
- Generated files combine:
  - Standard protobuf Go code (`.pb.go`)
  - Standard gRPC Go code (`_grpc.pb.go`)
  - Gorums-specific code (`_gorums.pb.go`)

### Custom Protobuf Options

Gorums provides custom protobuf options defined in `gorums.proto`:

- Method-level options for quorum call types
- Configuration options for RPC behavior
- See `doc/user-guide.md` for details

## Documentation

Maintained user and developer documentation belongs under `doc/`.
Private issues, designs, reviews, plans, specifications, and tasks belong under the ignored `.scratch/` directory.

Before making significant changes, consult:

- `doc/user-guide.md` - Understanding the API and usage patterns
- `doc/dev-guide.md` - Development workflow and architecture
- `.scratch/README.md`, when present - Private work index and local issue conventions
- `README.md` - Project overview and getting started
- When editing markdown files, use one sentence per line, so that diffs are easier to read.

## Common Pitfalls to Avoid

1. **Editing Generated Files Directly** - Always edit templates instead
2. **Skipping `make` After Changes** - Templates must be regenerated
3. **Breaking Backward Compatibility** - Require explicit approval from project maintainer
4. **Adding Unnecessary Features** - Follow YAGNI (You Aren't Gonna Need It)
5. **Ignoring Test Failures** - All tests must pass
6. **Inconsistent Code Style** - Match surrounding code style
7. **Poor Commit Hygiene** - Commit frequently with clear messages
8. **Committing Generated Files Together with Template Changes** - Always commit templates and static code separately. Only commit generated files as the last step.

## Performance Considerations

- Gorums is used in performance-critical distributed systems
- Profile before optimizing - use Go's pprof tools

## Communication with Project Maintainer

When uncertain:

- **STOP and ask** rather than making assumptions
- Clearly explain technical reasoning for design choices
- Be honest about limitations or lack of understanding
- Push back on bad ideas with technical justification
- Discuss architectural changes before implementing them

## Module and Dependency Management

- Uses Go modules (`go.mod`, `go.work`)
- Dependencies managed via `go mod tidy`
- Tool dependencies declared in `go.mod` tool section
- Examples have separate `go.mod` file

## Quality Standards

- **Correctness** over speed - take time to do it right
- **Simplicity** over cleverness - prefer maintainable solutions
- **Consistency** - match existing patterns and style in the codebase, call out deviations explicitly
- **Testing** - comprehensive test coverage required
- **Documentation** - keep docs in sync with code changes

## Additional Resources

- [gRPC Documentation](https://grpc.io/)
- [Protocol Buffers Guide](https://protobuf.dev/)
- [Go standard library documentation](https://pkg.go.dev/std)
- [Go effective practices](https://go.dev/doc/effective_go)
- [Go code review comments](https://go.dev/wiki/CodeReviewComments)
- [Go style guide](https://google.github.io/styleguide/go/guide)
- [Go best practices](https://google.github.io/styleguide/go/best-practices)
- [Go blog](https://go.dev/blog/)
- Project publications listed in README.md
