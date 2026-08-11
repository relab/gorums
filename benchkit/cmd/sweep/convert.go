package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/relab/gorums/benchkit"
	"google.golang.org/protobuf/encoding/protojson"
)

// convertBinaryResults writes a human-readable protojson ".json" sibling next
// to each collected ".binpb" file for a run, for manual inspection of a result.
// The binary file is kept as the source-of-truth artifact.
//
// Missing or undecodable files are skipped with a warning so a partial run still
// converts whatever it collected.
func convertBinaryResults(outdir, base string, nodes []nodeAssignment) {
	for _, node := range nodes {
		binPath := filepath.Join(outdir, resultFilename(base, node, resultExt))
		if err := convertBinaryFile(binPath); err != nil {
			log.Printf("  warning: convert: %v", err)
		}
	}
}

// convertDirBinaryResults writes a protojson ".json" sibling for every ".binpb"
// result file in dir, returning the number successfully converted. The laptop
// driver calls this after downloading a driven run, which ships only the binary
// results to keep the WAN transfer small; the readable protojson is
// regenerated here instead of crossing the WAN. Undecodable files are skipped
// with a warning so a partial download still converts whatever it has.
func convertDirBinaryResults(dir string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*"+resultExt))
	if err != nil {
		return 0, err
	}
	n := 0
	for _, binPath := range matches {
		if err := convertBinaryFile(binPath); err != nil {
			log.Printf("  warning: convert: %v", err)
			continue
		}
		n++
	}
	return n, nil
}

// convertBinaryFile decodes one ".binpb" result file and writes its protojson
// ".json" sibling. Decoding uses the same generated [benchkit.Report] type the
// benchmark wrote it with, so the protojson output matches what
// protojson.Marshal produces for that schema.
func convertBinaryFile(binPath string) error {
	data, err := os.ReadFile(binPath)
	if err != nil {
		return err
	}
	res, err := benchkit.DecodeReport(data)
	if err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(binPath), err)
	}
	jsonBytes, err := protojson.Marshal(res)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(binPath), err)
	}
	jsonPath := strings.TrimSuffix(binPath, resultExt) + ".json"
	if err := os.WriteFile(jsonPath, jsonBytes, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(jsonPath), err)
	}
	return nil
}
