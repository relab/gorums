package main

import (
	"reflect"
	"testing"
)

func TestNormalizeOptionalArgs(t *testing.T) {
	got := normalizeOptionalArgs([]string{"sweep", "-collect", "/local/a run", "-outdir", "out"})
	want := []string{"sweep", "-collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalArgs([]string{"sweep", "-collect-now", "-detach", "bb1"})
	want = []string{"sweep", "-collect-now", "-detach=bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	// The double-dash forms, which Go's flag package treats identically to
	// the single-dash forms, must be normalized the same way; otherwise
	// "--collect <path>" parses <path> as a bare boolean followed by a stray
	// positional argument, silently collecting the latest run instead of the
	// requested one (see the flag.NArg() check in main for the other half of
	// this fix).
	got = normalizeOptionalArgs([]string{"sweep", "--collect", "/local/a run", "-outdir", "out"})
	want = []string{"sweep", "--collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalArgs([]string{"sweep", "--collect-now", "-detach", "bb1"})
	want = []string{"sweep", "--collect-now", "-detach=bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
