package main

import (
	"reflect"
	"testing"
)

func TestNormalizeOptionalPathArgs(t *testing.T) {
	got := normalizeOptionalPathArgs([]string{"sweep", "-collect", "/local/a run", "-outdir", "out"})
	want := []string{"sweep", "-collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalPathArgs([]string{"sweep", "-collect-now", "-driver", "bb1"})
	want = []string{"sweep", "-collect-now", "-driver", "bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	// The double-dash forms, which Go's flag package treats identically to
	// the single-dash forms, must be normalized the same way; otherwise
	// "--collect <path>" parses <path> as a bare boolean followed by a stray
	// positional argument, silently collecting the latest run instead of the
	// requested one (see main.go's flag.NArg() check for the other half of
	// this fix).
	got = normalizeOptionalPathArgs([]string{"sweep", "--collect", "/local/a run", "-outdir", "out"})
	want = []string{"sweep", "--collect=/local/a run", "-outdir", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	got = normalizeOptionalPathArgs([]string{"sweep", "--collect-now", "-driver", "bb1"})
	want = []string{"sweep", "--collect-now", "-driver", "bb1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
