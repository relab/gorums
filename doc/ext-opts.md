### Adding a new extension option

1. Add your extension option to `gorums.proto`. We currently only have method options.
2. Run `make gorumsprotoopts` to regenerate the `gorums.pb.go` file. (TODO we could probably avoid using a make file for this and instead do `go generate`)
3. Add a check function, such as `hasPerNodeArgExtension()`, for your option in `plugins/gorums/ext.go`.
4. Update the `plugins/gorums/gorums.go` as follows 
   a. add the option `PerNodeArg` bool to the `serviceMethod` struct.
   b. add the option to initialize of the `serivceMethod` struct in the `verifyExtensionsAndCreate` function, like this: `PerNodeArg:        hasPerNodeArgExtension(method),`
   c. update the logic in the `isQuorumCallVariant` function if necessary.
   d. update the error handling logic in `verifyExtensionsAndCreate`.
5. Update the template files (`.tmpl` in `dev` folder) related to your option. This is were your on your own.
6. To regenerate the gorums plugin, you need to run `make goldenanddev` so that new proto files will understand your option.
7. Update the `dev/register.proto` file with your new option (probably on a new method).
8. To use the new option in the `dev/register.proto` file, you need to run `make devproto`.
