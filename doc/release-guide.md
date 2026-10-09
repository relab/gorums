# Releasing Gorums

Releases are made with the `gorums-release` program, in three steps: `prepare`, `pr`, and `publish`.
The root module and the `benchkit` module are released together with one version.
A release `vX.Y.Z` creates two tags on one commit: `vX.Y.Z` and `benchkit/vX.Y.Z`.
The `examples` module has no tag.

## Install

You need a logged-in GitHub CLI (`gh auth login`) and `protoc` on your `PATH`.
Install the program from a checkout of the repository:

```shell
go install ./internal/cmd/gorums-release
```

Run it inside a checkout.
Install it again after the program changes.

## Steps

1. Prepare the release.

   ```shell
   gorums-release prepare
   ```

   Run it on an up-to-date `master` with a clean tree.
   Afterward the working tree holds everything the release needs: the new version, upgraded dependencies, and regenerated code.
   The tests have passed.
   Nothing is committed.

2. Review the changes with `git diff`.

   `GenVersion` and `MinVersion` in `runtime/gorumsimpl/version.go` are never changed by the program.
   Check by hand whether the release needs new values.

3. Open the release pull request.

   ```shell
   gorums-release pr
   ```

   This creates the branch `release/vX.Y.Z` with two commits, one for the version and dependencies and one for the generated code, and opens the pull request.

4. Wait for CI to pass on the pull request.

5. Publish the release.

   ```shell
   gorums-release publish
   ```

   This merges the pull request, pushes both tags, creates the GitHub release, and checks that the Go module proxy serves the new version.

Each command lists what it does, step by step, under `gorums-release <command> -h`.

## Choosing the Version

By default, `prepare` uses the version that `gorelease` suggests.
To see the suggestion before you change anything, add `-dry-run`:

```shell
gorums-release prepare -dry-run
```

It prints the suggested version and an example command for another version.
Use `-version` to choose a version yourself:

```shell
gorums-release prepare -version v0.12.0-rc.1
```

A version with a suffix, such as `v0.12.0-rc.1` or `v1.0.0-rc.1`, is a pre-release, and the GitHub release is marked as one.
A version without a suffix is a normal release, also for `v0.X.Y`.
Gorums stays at `v0.X.Y` for now, so `prepare` stops if `gorelease` suggests `v1.0.0` or higher.
Pass `-version` to confirm such a version on purpose.

## Flags

Every command accepts `-dry-run`.
It shows what would be done and changes nothing.
A line that starts with `+` is a command or file change that is skipped.
A line that starts with `?` is a read-only query, such as `gorelease` or `git status`, that runs to plan the later steps.
`gorelease` needs a clean checkout, so `prepare -dry-run` works only on one.

These flags belong to one command:

| Command   | Flag            | Default                          | Meaning                                                    |
| --------- | --------------- | -------------------------------- | ---------------------------------------------------------- |
| `prepare` | `-version`      | the version `gorelease` suggests | Use this version, such as `v0.12.0` or `v0.12.0-rc.1`.     |
| `prepare` | `-skip-upgrade` | off                              | Do not upgrade dependencies. Use it when you run it again. |
| `prepare` | `-skip-tests`   | off                              | Do not run `make test` and `make testrace`.                |
| `pr`      | `-web`          | off                              | Open the pull request in the browser.                      |
| `publish` | `-yes`          | off                              | Merge the pull request without asking.                     |
| `publish` | `-draft`        | off                              | Create the GitHub release as a draft.                      |

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

## Version Constants

`internal/version/version.go` holds `Major`, `Minor`, `Patch`, and `PreRelease`.
`prepare` writes them.
`runtime/gorumsimpl/version.go` holds `MaxVersion`, `GenVersion`, and `MinVersion`.
`MaxVersion` follows the minor version.
You edit the other two by hand.
The compiler checks that `MinVersion ≤ GenVersion ≤ MaxVersion`.
