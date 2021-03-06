# Developer Guide

The repository contains two main components located in `cmd/protoc-gen-gorums`:

* **dev:** Contains static code and code generated by templates.
  In other words, this is the code that the `protoc-gen-gorums` compiler will output.
  All files that are not prefixed by `zorums*` and do not end in `_test.go` get bundled into `template_static.go`.
* **gengorums:** Contains the code for the `protoc-gen-gorums` compiler, as well as templates.

## Workflow and File Endings

1. **Template Changes:** Changes to generated code in the `dev/zorums_*_gorums.pb.go` files should be done by editing the various templates in the corresponding `gengorums/template_*.go` file.
   Making changes to the `zorums*` files will be overwritten.
   Note that there is not a one-to-one correspondence for all files.

2. **Static Code File Changes:** Changes to non-generated code in `dev/{config.go|mgr.go|node.go}` can be done in the respective `.go` files.

Any changes to templates or static code requires the invocation of `make` in order to:

* Bundle the static code into `template_static.go`.
* Rebuild the `protoc-gen-gorums` compiler.
* Update the `zorums_*_gorums.pb.go` files.

See the `Makefile` for more details.
To force compile, e.g. following a `protoc` update, you can use:

```sh
make -B
```

## Testing

Testing the `protoc-gen-gorums` compiler itself can be done by running the command below.
These tests verify that the code generated by the Gorums compiler is correct and stable.

```sh
make test
```

## Benchmarking

See [benchmarking.md](./benchmarking.md)

## Makefile

Below is a description of the current `Makefile` targets.
The `Makefile` itself also serves as documentation; inspect it for details.

| Target            | Description                                                                  |
| ----------------- | ---------------------------------------------------------------------------- |
| `dev`             | Updates `template_static.go` and regenerates generated files from templates. |
| `benchmark`       | Compiles the benchmark tool.                                                 |
| `tools`           | Installs required tools such as `protoc-gen-go` and `protoc-gen-go-grpc`.    |
| `installgorums`   | Reinstalls the `protoc-gen-gorums` plugin.                                   |
| `bootstrapgorums` | Used to bootstrap the plugin.                                                |
| `test`            | Run Gorums tests.                                                            |
