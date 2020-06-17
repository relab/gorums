package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/gorums/cmd/protoc-gen-gorums/gengorums"

	"google.golang.org/protobuf/compiler/protogen"
)

const bundleLen = len("--bundle=")

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintf(os.Stderr, "%v %v\n", filepath.Base(os.Args[0]), gengorums.VersionString())
		os.Exit(1)
	}
	if len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "--bundle=") {
		bundle := os.Args[1][bundleLen:]
		if bundle != "" {
			fmt.Fprintf(os.Stderr, "Generating bundle file: %s\n", bundle)
			gengorums.GenerateBundleFile(bundle)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "%v --bundle flag cannot be empty\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	var (
		flags flag.FlagSet
		dev   = flags.Bool("dev", false, "generate development files in dev folder")
		trace = flags.Bool("trace", false, "enable tracing")
		opts  = &protogen.Options{
			ParamFunc: flags.Set,
		}
	)

	opts.Run(func(gen *protogen.Plugin) error {
		if *trace {
			gengorums.SetTrace(*trace)
			fmt.Fprintf(os.Stderr, "Generating code with tracing enabled\n")
		}
		for _, f := range gen.Files {
			if f.Generate {
				switch {
				case *dev:
					fmt.Fprintf(os.Stderr, "Generating development files in dev folder\n")
					gengorums.GenerateDevFiles(gen, f)
				default:
					gengorums.GenerateFile(gen, f)
				}
			}
		}
		return nil
	})
}
