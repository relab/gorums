package main

import "testing"

func TestSemverParse(t *testing.T) {
	tests := []struct {
		in      string
		want    semver
		wantErr bool
	}{
		{in: "v0.12.0", want: semver{0, 12, 0, ""}},
		{in: "v1.2.3-rc.1", want: semver{1, 2, 3, "rc.1"}},
		{in: "v0.12", wantErr: true},
		{in: "0.12.0", wantErr: true},
		{in: "v0.12.x", wantErr: true},
		{in: "v0.12.0-", wantErr: true},
		{in: "v0.12.0+meta", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := parseSemver(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSemver(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseSemver(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
			if err == nil && got.String() != tt.in {
				t.Errorf("String() = %q, want %q", got.String(), tt.in)
			}
		})
	}
}

func TestSemverCompareCore(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.12.0", "v0.11.0", 1},
		{"v0.11.0", "v0.12.0", -1},
		{"v0.11.1", "v0.11.0", 1},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.12.0", "v0.12.0-rc.1", 0},
	}
	for _, tt := range tests {
		a, _ := parseSemver(tt.a)
		b, _ := parseSemver(tt.b)
		if got := a.compareCore(b); got != tt.want {
			t.Errorf("%s.compareCore(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
