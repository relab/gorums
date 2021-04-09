package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/gorums/cmd/protoc-gen-gorums/gengorums"
	"github.com/relab/gorums/internal/version"

	"google.golang.org/protobuf/compiler/protogen"
)

const (
	bundleLen       = len("--bundle=")
	genGorumsDocURL = "https://github.com/relab/gorums/blob/master/doc/user-guide.md"
	genGoDocURL     = "https://developers.google.com/protocol-buffers/docs/reference/go-generated"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Printf("%v %v\n", filepath.Base(os.Args[0]), version.String())
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == "--help" {
		fmt.Printf("See %s for usage information.\n", genGorumsDocURL)
		fmt.Printf("See %s for information about protobuf.\n", genGoDocURL)
		os.Exit(0)
	}
	if len(os.Args) == 2 && strings.HasPrefix(os.Args[1], "--bundle=") {
		bundle := os.Args[1][bundleLen:]
		if bundle != "" {
			fmt.Printf("Generating bundle file: %s\n", bundle)
			gengorums.GenerateBundleFile(bundle)
			os.Exit(0)
		}
		fmt.Printf("%v --bundle flag cannot be empty\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	var (
		flags flag.FlagSet
		dev   = flags.Bool("dev", false, "generate development files in dev folder")
		opts  = &protogen.Options{
			ParamFunc: flags.Set,
		}
	)

	opts.Run(func(gen *protogen.Plugin) error {
		for _, f := range gen.Files {
			if f.Generate {
				switch {
				case *dev:
					gengorums.GenerateDevFiles(gen, f)
				default:
					gengorums.GenerateFile(gen, f)
				}
			}
		}
		return nil
	})
}
