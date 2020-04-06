package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/relab/gorums/cmd/protoc-gen-gorums/gengorums"

	"google.golang.org/protobuf/compiler/protogen"
)

var devTypes = []string{
	"node",
	"qspec",
	"types",
	"qc",
	"qc_future",
	"correctable",
	"correctable_stream",
	"multicast",
	"strict_ordering_qc",
	"strict_ordering_rpc",
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Fprintf(os.Stderr, "%v %v\n", filepath.Base(os.Args[0]), gengorums.VersionString())
		os.Exit(1)
	}

	var (
		flags  flag.FlagSet
		dev    = flags.Bool("dev", false, "generate development files in dev folder")
		trace  = flags.Bool("trace", false, "enable tracing")
		bundle = flags.String("bundle", "", "generate static gorums code in given file")
		opts   = &protogen.Options{
			ParamFunc: flags.Set,
		}
	)

	protogen.Run(opts, func(gen *protogen.Plugin) error {
		if *trace {
			gengorums.SetTrace(*trace)
			fmt.Fprintf(os.Stderr, "Generating code with tracing enabled\n")
		}
		for _, f := range gen.Files {
			if f.Generate {
				switch {
				case *dev:
					fmt.Fprintf(os.Stderr, "Generating development files in dev folder\n")
					for _, gorumsType := range devTypes {
						gengorums.GenerateDevFile(gorumsType, gen, f)
					}
				case *bundle != "":
					fmt.Fprintf(os.Stderr, "Generating bundle file: %s\n", *bundle)
					gengorums.GenerateBundleFile(*bundle)
				default:
					gengorums.GenerateFile(gen, f)
				}
			}
		}
		return nil
	})
}
