package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunnerStdin(t *testing.T) {
	c := cmd{name: "cat", stdin: "secret line\n"}
	if strings.Contains(c.String(), "secret") {
		t.Errorf("String() = %q, want it to leave out stdin", c.String())
	}
	var stdout, stderr bytes.Buffer
	r := execRunner{root: t.TempDir(), stdout: &stdout, stderr: &stderr}
	out, err := r.Query(c)
	if err != nil {
		t.Fatal(err)
	}
	if out != "secret line\n" {
		t.Errorf("Query() = %q, want the stdin text", out)
	}
	if err := r.Exec(c); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "secret line\n" {
		t.Errorf("Exec() wrote %q, want the stdin text", stdout.String())
	}
}
