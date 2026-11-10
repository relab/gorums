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
A new driver also needs a new key line on the jump host: change the driver in `internal/cmd/gorums-release/clusterkey.go` too, and run `gorums-release cluster-key` again.
The pool needs at least 9 hosts, because the smoke sweep runs `-test 9`, and every host in it must be reachable.

### Set Up Access

Do this once.

1. Get the account `gorums` on `ssh4.ux.uis.no` and on the cluster, with one home directory on the jump host and another, shared one on the cluster hosts.
   You must be able to log in to it with `ssh gorums@ssh4.ux.uis.no` and `ssh gorums@bb1`, with a password or your own key.

2. Run `cluster-key`, logged in to `gh` as a repository admin:

   ```shell
   gorums-release cluster-key
   ```

   It creates a key pair, with today's date in the key's comment.
   It adds the public key to `~/.ssh/authorized_keys` of the account `gorums`:

   ```text
   ssh4.ux.uis.no:  command="/bin/false",restrict,port-forwarding,permitopen="bb1.ux.uis.no:22" <public key>
   cluster hosts:   from="152.94.1.60,152.94.162.0/26",restrict,port-forwarding,agent-forwarding,pty <public key>
   ```

   On the jump host, the key can only forward SSH to the driver.
   On the cluster, the key works only from the jump host and the cluster network.
   Then it creates the environment `cluster`, with you as required reviewer and only `release/v*` branches admitted, and sets its secret `CLUSTER_SSH_KEY` to the private key.
   Last, it requires approval of workflows from all outside collaborators.
   No copy of the private key stays on your machine.
   Use `-jump` and `-cluster` if your SSH config reaches the account under other names.

3. Check the access with the `cluster-check` workflow.
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

1. Run `gorums-release cluster-key`.
   It adds the new key next to the old one and replaces the secret.
2. Run the `cluster-check` workflow as in step 3.
3. Remove the old key from the jump host and the cluster:

   ```shell
   gorums-release cluster-key -prune
   ```
