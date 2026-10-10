# Releasing Gorums

Releases are made with the `gorums-release` program.
You choose the versions, then run three commands: `prepare`, `pr`, and `publish`.
The root module and the `benchkit` module are released together with one version.
A release `vX.Y.Z` creates two tags on one commit: `vX.Y.Z` and `benchkit/vX.Y.Z`.
The `examples` module has no tag.

## Requirements

You need these programs on your `PATH`:

- `go`, at the version that `go.mod` asks for;
- `git`, `make`, and `protoc`;
- the GitHub CLI `gh`, logged in with `gh auth login`.

The directory where `go install` puts programs must also be on your `PATH`.
It is `$(go env GOBIN)`, or `$(go env GOPATH)/bin` if `GOBIN` is empty.
The program installs `gorelease` and the code generators there, and `protoc` finds the generators by `PATH`.
You need permission to push branches and merge pull requests in `relab/gorums`.

`prepare` checks all of this and says what is missing.

## Install

Install the program from a checkout of the repository:

```shell
go install ./internal/cmd/gorums-release
```

Run it inside a checkout.
Install it again after the program changes.

## Steps

1. Choose the versions.

   Start from an up-to-date `master` with a clean tree and no untracked files.
   Run a dry run to see what `gorelease` suggests:

   ```shell
   gorums-release prepare -dry-run
   ```

   A line that starts with `+` is a command or file change that is skipped.
   A line that starts with `?` is a read-only query that runs to plan the later steps.
   `gorelease` needs a clean checkout, so the dry run works only on one.

   1. Choose the release version.

      By default, `prepare` uses the version that `gorelease` suggests.
      The dry run prints an example for another version.
      Use `-version` to choose one yourself, such as `-version v0.12.0-rc.1`.
      A version with a suffix is a pre-release, and the GitHub release is marked as one.
      A version without a suffix is a normal release, also for `v0.X.Y`.

      A major version bump, such as `v0.12.0` to `v1.0.0`, stops unless you also pass `-allow-major`.
      This holds for a suggested version and for `-version`.
      A bump to `v2.0.0` or higher is refused, because it needs a new module path (`github.com/relab/gorums/v2`) that the program cannot create.
      `MaxVersion` is the minor version, so a bump that resets the minor below `GenVersion` is refused too, until the runtime version scheme is changed.

   2. Decide whether the runtime versions change.

      The generated code checks at compile time that it fits the runtime: `MinVersion ≤ GenVersion ≤ MaxVersion`.
      These constants are in `runtime/gorumsimpl/version.go`.
      `GenVersion` is written into every generated file.
      `MaxVersion` is the minor version of the release, so it rises by itself.
      `MinVersion` and `GenVersion` change only when the generated code and the runtime stop fitting together:

      | Change in this release                                                 | Flag        | Result                                                                                                                                 |
      | ---------------------------------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------- |
      | The generator emits code that needs a runtime feature added now        | `-bump-gen` | `GenVersion` becomes the new minor. Newly generated code needs this runtime or newer. Code from older releases still compiles.         |
      | The runtime drops or changes something that older generated code calls | `-bump-min` | `MinVersion` and `GenVersion` become the new minor. Code from older releases stops compiling, and users must generate it again.        |
      | Neither                                                                | none        | Both values stay as they are.                                                                                                          |

      Without a flag, the dry run prints a hint when the generator output changed since the last release.
      Decide now: the generated code contains `GenVersion`, so `prepare` sets the values before it regenerates the code.
      To change your mind afterward, undo the changes (see below) and run `prepare` again.
      The values can only be set to the new minor version, so in a patch release `-bump-gen` changes nothing.

2. Prepare the release, with the flags you chose.

   ```shell
   gorums-release prepare
   ```

   Afterward the working tree holds everything the release needs: the new version, the runtime versions, upgraded dependencies, and regenerated code.
   The tests have passed.
   Nothing is committed.

3. Review the changes with `git diff`.

   `prepare` prints the final `GenVersion`, `MinVersion`, and `MaxVersion`.
   Check that they are what you decided in step 1.

4. Open the release pull request.

   ```shell
   gorums-release pr
   ```

   This creates the branch `release/vX.Y.Z` with two commits, one for the version and dependencies and one for the generated code, and opens the pull request.

5. Wait for CI to pass on the pull request.

6. Publish the release.

   ```shell
   gorums-release publish
   ```

   This merges the pull request, pushes both tags, creates the GitHub release, and checks that the Go module proxy serves the new version.

Each command lists what it does, step by step, under `gorums-release <command> -h`.

## Flags

Every command accepts `-dry-run`.
These flags belong to one command:

| Command   | Flag            | Default                          | Meaning                                                                      |
| --------- | --------------- | -------------------------------- | ---------------------------------------------------------------------------- |
| `prepare` | `-version`      | the version `gorelease` suggests | Use this version, such as `v0.12.0` or `v0.12.0-rc.1`.                       |
| `prepare` | `-allow-major`  | off                              | Confirm a major version bump, such as `v0` to `v1`.                          |
| `prepare` | `-bump-gen`     | off                              | Set `GenVersion` to the new minor version.                                   |
| `prepare` | `-bump-min`     | off                              | Set `MinVersion` and `GenVersion` to the new minor version.                  |
| `prepare` | `-skip-upgrade` | off                              | Do not upgrade dependencies. Use it when you run it again.                   |
| `prepare` | `-skip-tests`   | off                              | Do not run `make test` and `make testrace`.                                  |
| `pr`      | `-web`          | off                              | Open the pull request in the browser.                                        |
| `publish` | `-yes`          | off                              | Merge the pull request without asking.                                       |
| `publish` | `-draft`        | off                              | Create the GitHub release as a draft.                                        |

## If Something Goes Wrong

- If `prepare` fails, fix the cause and undo its changes before you run it again.
  Run `git restore .` for the changed files.
  Run `git clean -nd` to list the new files that code generation added, and `git clean -fd` to delete them.
  `prepare` refuses to start when untracked files exist.
- If `pr` stops because `gorelease` rejects the version, the branch `release/vX.Y.Z` has local commits and nothing is pushed.
  Switch back to `master`, delete the branch, undo the changes as above, and run `prepare` with another `-version`.
- If `publish` stops after it pushed the tags, finish by hand with `gh release create vX.Y.Z --generate-notes`.
  Never move or delete a tag that the Go module proxy has served.
- If the proxy check times out, the release is still complete.
  Check later with `go list -m github.com/relab/gorums@vX.Y.Z` and `go list -m github.com/relab/gorums/benchkit@vX.Y.Z`.
