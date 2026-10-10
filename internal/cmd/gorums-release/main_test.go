package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"no command", nil, 2, "", "Usage: gorums-release"},
		{"help", []string{"help"}, 0, "Usage: gorums-release", ""},
		{"unknown", []string{"ship"}, 2, "", `unknown command "ship"`},
		{"extra argument", []string{"pr", "v1"}, 2, "", `unexpected argument "v1"`},
		{"unknown flag", []string{"prepare", "-nope"}, 2, "", "flag provided but not defined"},
		{"prepare help", []string{"prepare", "-h"}, 0, "", "runtime/gorumsimpl/version.go"},
		{"publish help", []string{"publish", "-h"}, 0, "", "-draft"},
		{"pr help", []string{"pr", "-h"}, 0, "", "-web"},
		{"prepare bump flag", []string{"prepare", "-h"}, 0, "", "-bump-min"},
		{"prepare major flag", []string{"prepare", "-h"}, 0, "", "-allow-major"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			if got := run(tt.args, strings.NewReader(""), &out, &errOut); got != tt.wantCode {
				t.Errorf("run() = %d, want %d", got, tt.wantCode)
			}
			if !strings.Contains(out.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want it to contain %q", out.String(), tt.wantOut)
			}
			if !strings.Contains(errOut.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errOut.String(), tt.wantErr)
			}
		})
	}
}
