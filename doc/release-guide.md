# Preparing a new Release of Gorums

Below are the steps to prepare a new release of Gorums.

1. Check and upgrade dependencies:

   ```shell
   % protoc --version
   libprotoc 3.15.6
   # v3.15.6 is current; but if new version available run:
   % brew upgrade protobuf
   % protoc-gen-go-grpc --version
   protoc-gen-go-grpc 1.1.0
   % protoc-gen-go --version
   protoc-gen-go v1.25.0
   # v1.25.0 is old; run make tools to upgrade
   % make tools
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
   % go install github.com/relab/gorums/cmd/protoc-gen-gorums
   % protoc-gen-gorums --version
   protoc-gen-gorums v0.4.0-devel
   ```

6. Recompile `_gorums.pb.go` files:

   ```shell
   % make -B
   % cd examples
   % make -B
   % go mod tidy
   ```

7. Run tests:

   ```shell
   % make test
   % make testrace
   ```

8. Add and commit changes due to upgrades and recompilation:

   ```shell
   % git add
   % git commit -m "Gorums release v0.4.0"
   ```

9. Tag and push the release:

   ```shell
   % git tag v0.4.0
   % git push origin v0.4.0
   # Synchronize master branch with v0.4.0
   % git push
   ```

   Now other projects can depend on `v0.4.0` of `github.com/relab/gorums`.

10. To check that the new version is available (after a bit of time):

    ```shell
    % go list -m github.com/relab/gorums@v0.4.0
    ```
