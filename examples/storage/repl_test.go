package main

import (
	"reflect"
	"testing"
)

func TestSplitQuoted(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{`qc read foo`, []string{"qc", "read", "foo"}},
		{`cfg 1:3 write foo bar`, []string{"cfg", "1:3", "write", "foo", "bar"}},
		{`cfg 0,2 write foo 'bar baz'`, []string{"cfg", "0,2", "write", "foo", "bar baz"}},
		{`qc write k ""`, []string{"qc", "write", "k", ""}}, // This is the failing case
		{`qc write k ''`, []string{"qc", "write", "k", ""}}, // Single quotes too
		{`qc write "" k`, []string{"qc", "write", "", "k"}},
		{`""`, []string{""}},
		{`"" ""`, []string{"", ""}},
		{`'' ''`, []string{"", ""}},
		{`  ""  `, []string{""}},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			actual, err := splitQuoted(test.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(actual, test.expected) {
				t.Errorf("expected %v, got %v", test.expected, actual)
			}
		})
	}
}
