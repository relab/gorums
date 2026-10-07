package main

import "testing"

func TestValidateStreamModes(t *testing.T) {
	tests := []struct {
		name    string
		modes   []string
		binary  string
		wantErr bool
	}{
		{name: "Empty", modes: nil},
		{name: "Dual", modes: []string{"dual"}},
		{name: "DualAndDedup", modes: []string{"dual", "dedup"}},
		{name: "Invalid", modes: []string{"bogus"}, wantErr: true},
		{name: "BaselineWithBinary", modes: []string{"baseline"}, binary: "/tmp/bench"},
		{name: "BaselineWithoutBinary", modes: []string{"baseline"}, wantErr: true},
		{name: "BaselineMixed", modes: []string{"baseline", "dual"}, binary: "/tmp/bench", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStreamModes(tt.modes, tt.binary)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateStreamModes(%v, %q) = %v, wantErr %v", tt.modes, tt.binary, err, tt.wantErr)
			}
		})
	}
}
