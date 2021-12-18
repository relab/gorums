# Preparing a new Release of Gorums

Below are the steps to prepare a new release of Gorums.

To cut a release you will need additional tools:

```shell
% go install golang.org/x/exp/cmd/gorelease@latest
% brew install gh
```

1. Check and upgrade dependencies:

   ```shell
   % make tools
   % protoc --version
   libprotoc 3.15.6
   # v3.15.6 is current; but if new version available run:
   % brew upgrade protobuf
   % protoc-gen-go-grpc --version
   protoc-gen-go-grpc 1.1.0
   % protoc-gen-go --version
   protoc-gen-go v1.26.0
   # Upgrade module dependencies
   % go get -u ./...
   % cd examples
   % go get -u ./...
   % cd ..
   ```

2. Run `gorelease` to suggested new version number, e.g.:

   ```text
   ... (list of compatability changes) ...
   Inferred base version: v0.3.0
   Suggested version: v0.4.0
   ```

3. Edit `internal/version/version.go`

4. Edit `version.go`

5. Install new version of `protoc-gen-gorums`:

   ```shell
   % make dev
   % protoc-gen-gorums --version
   protoc-gen-gorums v0.4.0-devel
   ```

6. Recompile `_gorums.pb.go` files:

   ```shell
   % make -B
   % go mod tidy
   % cd examples
   % make -B
   % go mod tidy
   % cd ..
   ```

7. Run tests:

   ```shell
   % make test
   % make testrace
   ```

8. Edit gorums dependency to be v0.4.0 in example/go.mod:

   ```shell
   % vim examples/go.mod
   ```

9. Add and commit changes due to upgrades and recompilation:

   ```shell
   % git add
   % git commit -m "Gorums release v0.4.0"
   # Synchronize master branch
   % git push
   ```

10. Publish the release with release notes:

   ```shell
   # Prepare release notes in release-notes.md
   % gh release create v0.4.0 --prerelease -F release-notes.md --title "Main changes in release"
   ```

   Without the `gh` tool:

   ```shell
   % git tag v0.4.0
   % git push origin v0.4.0
   ```

   Now other projects can depend on `v0.4.0` of `github.com/relab/gorums`.

11. To check that the new version is available (after a bit of time):

    ```shell
    % go list -m github.com/relab/gorums@v0.4.0
    ```
