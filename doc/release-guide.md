# Releasing Gorums

Releases are made with the `gorums-release` program.
You choose the versions, then run three commands: `prepare`, `pr`, and `publish`.
The root module and the `benchkit` module are released together with one version.
A release `vX.Y.Z` creates two tags on one commit: `vX.Y.Z` and `benchkit/vX.Y.Z`.
The `examples` module has no tag.

## Requirements

You need these programs on your `PATH`:

- `go`, at the version that `go.mod` asks for;
- `git`, [`just`](https://github.com/casey/just), and `protoc`;
- the GitHub CLI `gh`, logged in with `gh auth login`.

The directory where `go install` puts programs must also be on your `PATH`.
It is `$(go env GOBIN)`, or `$(go env GOPATH)/bin` if `GOBIN` is empty.
The program installs `gorelease` there.
The `Justfile` runs the code generators with `go tool`, so they need no installation.
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
      `GenVersion` is written into the version check of every generated `_gorums.pb.go` file.
      The `gorums.pb.go` file of a package carries the version header lines but no version check.
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
   It also adds `replace github.com/relab/gorums vX.Y.Z => ./` to `go.work`, because the workspace cannot resolve a version that is not tagged yet.
   The replace is in `go.work`, not in a `go.mod`, because `go install pkg@version` rejects a module whose `go.mod` has a replace.
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

6. Approve and wait for the smoke checks.

   The push of the release branch starts the `release-smoke` workflow.
   It runs four jobs:

   - `guard` checks that the branch is `release/<version>` for the version in `version.go`;
   - `local-checks` runs `just integrationtest`, `just stresstest`, `just lint`, `just modernize` (which must change nothing), and `just goplscheck`;
   - `docker-smoke` runs `just smoke-docker`, a multi-host sweep on Docker containers;
   - `cluster-smoke` runs the same smoke sweep on the bbchain cluster, and checks it with `sweep -verify`.

   `cluster-smoke` waits for your approval of the `cluster` environment.
   Approve it in the Actions tab of the run (Review deployments).
   While it waits, a run for a newer push to the same branch waits too, so approve or reject it before you push again.
   It needs the cluster access described in [Cluster Access](#cluster-access).

   Each smoke job writes a verdict to its job summary.
   The verdict lists each failed criterion, with the error of a failed run and the degraded nodes of a degraded run.
   Each smoke job also uploads an artifact with its verdict and its full run directory, including `agg.csv`, the logs, and the raw results.

   `publish` refuses to merge until all three of `local-checks`, `docker-smoke`, and `cluster-smoke` have succeeded on the pull request.

7. Publish the release.

   ```shell
   gorums-release publish
   ```

   This merges the pull request, pushes both tags on its merge commit, creates the GitHub release, and checks that the Go module proxy serves the new version.
   The tags always name the merge commit of the release pull request, never the current `HEAD`, so commits that are not part of the release cannot end up under a release version.

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
| `prepare` | `-skip-tests`   | off                              | Do not run `just test` and `just testrace`.                                  |
| `pr`      | `-web`          | off                              | Open the pull request in the browser.                                        |
| `publish` | `-yes`          | off                              | Merge the pull request without asking.                                       |
| `publish` | `-draft`        | off                              | Create the GitHub release as a draft.                                        |

## If Something Goes Wrong

- If a smoke job fails, read its verdict in the job summary.
  Fix the cause on the release branch and push; the workflow runs again.
  To run a job again without a change, use "Re-run failed jobs" in the Actions tab.
- If the cluster is down and the release cannot wait, merge the release pull request by hand in GitHub.
  `publish` then publishes the merged pull request and prints a warning that names each smoke check that did not pass.
  Use this only for an outage, and run the cluster smoke check again once the cluster is back.
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

## Cluster Access

The `cluster-smoke` job of the `release-smoke` workflow runs a smoke sweep on the bbchain cluster from a GitHub-hosted runner.
It reaches the cluster over SSH through the UiS jump host, as the dedicated account `gorums`:

```text
GitHub runner ──ssh, ProxyJump only──► ssh4.ux.uis.no ──► driver (bb1) ──ssh──► pool hosts (bbN)
                                       shared jump host;     runs sweep -driven;  run the benchmark
                                       nothing runs here     holds /tmp/sweep-gorums
```

`ssh4.ux.uis.no` serves all UiS users, so it only forwards connections.
No sweep, binary, or file is ever placed there.

The runner's SSH config, the pinned host keys, the driver, and the pool are in `.github/actions/cluster-ssh/`.
To change the driver or the pool, edit those files in a pull request.
The pool needs at least 9 hosts, because the smoke sweep runs `-test 9`, and every host in it must be reachable.

### Set Up Access

Do this once.
The account `gorums` must have one home directory on `ssh4.ux.uis.no` and another, shared one on the cluster hosts.

1. Create a key pair for the workflow, without a passphrase.
   Put the year in its comment, so that its age shows in `authorized_keys`:

   ```shell
   ssh-keygen -t ed25519 -C gorums-release-smoke-2026 -N '' -f gorums-release-smoke
   ```

2. Install the public key for the account `gorums`, with one line on the jump host and one on the cluster.

   In `~/.ssh/authorized_keys` on `ssh4.ux.uis.no`:

   ```text
   command="/bin/false",restrict,port-forwarding,permitopen="bb1.ux.uis.no:22" <contents of gorums-release-smoke.pub>
   ```

   In `~/.ssh/authorized_keys` on the cluster hosts:

   ```text
   from="152.94.1.60,152.94.162.0/26",restrict,port-forwarding,agent-forwarding,pty <contents of gorums-release-smoke.pub>
   ```

   - On the jump host, the key can only forward SSH to the driver.
     Any command runs `/bin/false` instead; `ProxyJump` still works.
     The `permitopen` host must match the driver's `HostName` in `.github/actions/cluster-ssh/ssh_config`.
   - On the cluster, the key works only from the jump host (`152.94.1.60`) and the cluster network.
     Use addresses, not names.

3. In the repository settings, create the environment `cluster`:
   - deployment branches: only the pattern `release/v*`;
   - required reviewer: the maintainer;
   - the secret `CLUSTER_SSH_KEY`: the private key `gorums-release-smoke` from step 1.

   Then delete the private key file.

4. In the repository settings under Actions, require approval for workflows from all outside collaborators.

5. Check the access with the `cluster-check` workflow.
   Push any commit to a branch named `release/v0-probe`, approve the `cluster` environment in the Actions tab, and wait for the job:

   ```shell
   git push origin HEAD:refs/heads/release/v0-probe
   ```

   The job first checks that the jump host runs no command for the account: `ssh jump true` must exit with the status of `/bin/false`.
   It then runs `sweep -check -strict` from the driver.
   It passes when every host is reachable, no benchmark port is busy, and no benchmark process is left over.
   Then delete the branch:

   ```shell
   git push origin --delete release/v0-probe
   ```

The key reaches only jobs that name the `cluster` environment, run from a `release/v*` branch, and are approved.
Only people with write access can push such a branch, and a pull request from a fork runs without the secret.

### Rotate the Key

Rotate the key once a year.
Rotate it at once if it may have leaked, or if someone with admin access to the repository leaves.

1. Create a new key pair as in step 1, with the current year in its comment.
2. Add the new public key next to the old one, as in step 2.
3. Replace the secret `CLUSTER_SSH_KEY`, and run the `cluster-check` workflow as in step 5.
4. Remove the old key's lines from `authorized_keys` on the jump host and on the cluster.
