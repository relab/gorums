# Agent Instructions for Gorums Project

You are working on the Gorums project - a framework for building fault-tolerant distributed systems using quorum-based abstractions. This document provides critical context and rules for AI coding assistants.

## Project Overview

Gorums is a Go-based framework that provides:

- Flexible quorum call abstractions for distributed systems
- Code generation via `protoc-gen-gorums` compiler plugin
- gRPC-based RPC communication
- Support for various quorum patterns (multicast, unicast, quorumcall, correctable)

**Key Technologies:**

- Language: Go 1.25+
- Build: Make
- Protocol: Protocol Buffers (protobuf)
- RPC: gRPC
- Testing: Go testing framework
- Code Generation: Custom protoc plugin

## Repository Structure

```text
gorums/
├── cmd/protoc-gen-gorums/     # Compiler plugin for code generation
│   ├── dev/                   # Static code + generated code examples
│   └── gengorums/             # Compiler logic + templates
├── benchmark/                 # Benchmarking code
├── examples/                  # Example implementations
├── internal/                  # Internal packages
├── doc/                       # Documentation
└── *.go                       # Core library files
```

## Critical Development Rules

### Code Generation Workflow

**NEVER directly edit files prefixed with `zorums_*_gorums.pb.go` in `cmd/protoc-gen-gorums/dev/`**

These files are generated from templates. Instead:

1. **For Template Changes:**
   - Edit the corresponding template in `cmd/protoc-gen-gorums/gengorums/template_*.go`
   - Run `make` to regenerate code
   - The `zorums_*` files will be automatically updated

2. **For Static Code Changes:**
   - Edit files in `cmd/protoc-gen-gorums/dev/` that are NOT prefixed with `zorums_*`
   - Run `make` to bundle changes into `template_static.go`
   - Changes propagate to generated code automatically

3. **After Any Template or Static Code Changes:**

   ```bash
   make          # Normal rebuild
   make -B       # Force rebuild (e.g., after protoc update)
   ```

### Testing Requirements

- **Run tests after every change:** `make test`
- Tests verify the correctness and stability of generated code
- ALL test failures must be addressed before considering work complete
- Never delete failing tests - fix the underlying issue

### Code Style and Conventions

- **Match existing code style** - consistency within files is paramount
- **Follow Go conventions** - use `gofmt`, follow effective Go practices
- **Use Go's standard library**
  - use up-to-date standard library features when relevant
  - use recent versions of packages: slices, maps, sync, rand/v2
  - use for-range iterators with yield when applicable
  - use generics when appropriate
- **Use meaningful names** - reflect domain concepts, not implementation details
- **Preserve comments** unless they are demonstrably incorrect
- **Add documentation**
  - Each exported function, type, and method must have a clear comment explaining its purpose and usage following Go doc conventions
  - Non-exported functions, types, and methods should have comments if their purpose is not immediately clear
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

## Building and Testing

### Common Commands

```bash
# Build everything
make

# Force rebuild
make -B

# Run tests
make test

# Install protoc-gen-gorums plugin
make installgorums

# Build benchmark tool
make benchmark

# Install required tools
make tools
```

### Testing Strategy

- Always write table-driven tests when same logic needs to be tested with multiple inputs
- Follow Test Driven Development (TDD) when adding features:
  1. Write failing test
  2. Confirm test fails
  3. Write minimal code to pass test
  4. Confirm test passes
  5. Refactor if needed
- Test coverage should be comprehensive
- Never mock behavior in end-to-end tests
- Test output must be clean - no unexpected errors or warnings

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
- See `doc/method-options.md` for details

## Documentation

Before making significant changes, consult:

- `doc/user-guide.md` - Understanding the API and usage patterns
- `doc/dev-guide.md` - Development workflow and architecture
- `doc/design-doc-layering.md` - System architecture and layering
- `doc/method-options.md` - Protocol buffer options reference
- `README.md` - Project overview and getting started

## Common Pitfalls to Avoid

1. **Editing Generated Files Directly** - Always edit templates instead
2. **Skipping `make` After Changes** - Templates must be rebundled/regenerated
3. **Breaking Backward Compatibility** - Require explicit approval from project maintainer
4. **Adding Unnecessary Features** - Follow YAGNI (You Aren't Gonna Need It)
5. **Ignoring Test Failures** - All tests must pass
6. **Inconsistent Code Style** - Match surrounding code style
7. **Poor Commit Hygiene** - Commit frequently with clear messages
8. **Committing Generated Files Together with Template Changes** - Always commit templates and static code separately. Only commit generated files as the last step.

## Performance Considerations

- Gorums is used in performance-critical distributed systems
- Benchmarking tools are available in `benchmark/` and `cmd/benchmark/`
- See `doc/benchmarking.md` for benchmarking procedures
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
