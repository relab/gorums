package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

// gorumsPlotLib is the Typst helper library, embedded so the binary is
// self-contained. It is copied verbatim into each report directory next to the
// generated report.typ, which imports it by name.
//
//go:embed typst/gorumsplot.typ
var gorumsPlotLib string

const reportLibName = "gorumsplot.typ"

// copyReportLib writes the embedded Typst helper library into dir so the
// generated report.typ can import it by name and compile on any machine with
// Typst installed, independent of the repository.
func copyReportLib(dir string) error {
	return os.WriteFile(filepath.Join(dir, reportLibName), []byte(gorumsPlotLib), 0o644)
}
