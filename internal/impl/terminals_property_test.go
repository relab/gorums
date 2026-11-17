package impl

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/relab/gorums/internal/conn"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
	"pgregory.net/rapid"
)

var errPropertyNode = errors.New("property node error")

// responseList deliberately uses a small ID and value space to generate
// repeated contributors and competing agreements.
var responseList = rapid.SliceOfN(rapid.Custom(func(t *rapid.T) NodeResponse[*pb.Int32Value] {
	r := NodeResponse[*pb.Int32Value]{
		NodeID: rapid.Uint32Range(1, 8).Draw(t, "node"),
		Value:  pb.Int32(rapid.Int32Range(0, 3).Draw(t, "value")),
	}
	switch rapid.IntRange(0, 4).Draw(t, "outcome") {
	case 1:
		r.Err = errPropertyNode
	case 2:
		r.Err = fmt.Errorf("transform: %w", ErrSkipNode)
	}
	return r
}), 0, 40)

// finiteResponses counts yields so a terminal must stop at the correct prefix.
func finiteResponses(rows []NodeResponse[*pb.Int32Value], consumed *int) *Responses[*pb.Int32Value] {
	return &Responses[*pb.Int32Value]{size: 8, seq: func(yield func(NodeResponse[*pb.Int32Value]) bool) {
		for _, r := range rows {
			*consumed++
			if !yield(r) {
				return
			}
		}
	}}
}

// contributorIndices derives the successful contributors independently of the
// terminal implementation. distinct selects first-success-per-node semantics.
func contributorIndices(rows []NodeResponse[*pb.Int32Value], distinct bool) []int {
	var indices []int
	seen := map[ID]bool{}
	for i, r := range rows {
		if r.Err == nil && (!distinct || !seen[r.NodeID]) {
			indices = append(indices, i)
			seen[r.NodeID] = true
		}
	}
	return indices
}

func checkTerminalError(t *rapid.T, err error, complete bool, prefix []NodeResponse[*pb.Int32Value]) {
	t.Helper()
	if complete {
		if err != nil {
			t.Fatalf("completed terminal: %v", err)
		}
		return
	}
	var qerr conn.QuorumCallError
	if !errors.Is(err, ErrIncomplete) || !errors.As(err, &qerr) {
		t.Fatalf("incomplete terminal: %v", err)
	}
	var want []ID
	for _, r := range prefix {
		if r.Err != nil && !errors.Is(r.Err, ErrSkipNode) {
			want = append(want, r.NodeID)
		}
	}
	var got []ID
	for id, cause := range qerr.NodeErrors() {
		got = append(got, id)
		if !errors.Is(cause, errPropertyNode) {
			t.Fatalf("node error = %v", cause)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("node errors = %v, want %v", got, want)
	}
}

func TestTerminalsProperties(t *testing.T) {
	rapid.Check(t, checkTerminals)
}

// The same structured property is available to the coverage-guided fuzzer.
func FuzzTerminals(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	f.Fuzz(rapid.MakeFuzz(checkTerminals))
}

func checkTerminals(t *rapid.T) {
	rows := responseList.Draw(t, "responses")
	n := rapid.IntRange(1, 10).Draw(t, "threshold")
	distinct := contributorIndices(rows, true)
	all := contributorIndices(rows, false)

	// Threshold counts responses and returns the first successful value.
	consumed := 0
	value, err := finiteResponses(rows, &consumed).Threshold(n)
	wantConsumed := len(rows)
	if len(all) >= n {
		wantConsumed = all[n-1] + 1
	}
	var wantValue *pb.Int32Value
	if len(all) > 0 {
		wantValue = rows[all[0]].Value
	}
	if consumed != wantConsumed || value != wantValue {
		t.Fatalf("Threshold = (%v, %d yields), want (%v, %d)", value, consumed, wantValue, wantConsumed)
	}
	checkTerminalError(t, err, len(all) >= n, rows[:wantConsumed])

	// Matching picks the first key to obtain n distinct successful contributors.
	var want []NodeResponse[*pb.Int32Value]
	wantConsumed = len(rows)
	for pos, index := range distinct {
		var matching []NodeResponse[*pb.Int32Value]
		for _, earlier := range distinct[:pos+1] {
			if rows[earlier].Value.GetValue() == rows[index].Value.GetValue() {
				matching = append(matching, rows[earlier])
			}
		}
		if len(matching) == n {
			want, wantConsumed = matching, index+1
			break
		}
	}
	consumed = 0
	agreement, err := finiteResponses(rows, &consumed).Matching(n, func(v *pb.Int32Value) int32 { return v.GetValue() })
	wantValue = nil
	if len(want) > 0 {
		wantValue = want[0].Value
	}
	if consumed != wantConsumed || agreement.Value != wantValue || !slices.Equal(agreement.Responses, want) {
		t.Fatalf("Matching = (%v, %v, %d yields), want (%v, %v, %d)", agreement.Value, agreement.Responses, consumed, wantValue, want, wantConsumed)
	}
	checkTerminalError(t, err, len(want) == n, rows[:wantConsumed])

	// Reduce retains the accumulator returned by an early failing callback.
	failAt := rapid.IntRange(0, n+1).Draw(t, "callbackFailure")
	foldCount := min(n, len(distinct))
	fails := failAt > 0 && failAt <= foldCount
	if fails {
		foldCount = failAt
	}
	want = nil
	for _, i := range distinct[:foldCount] {
		want = append(want, rows[i])
	}
	wantConsumed = len(rows)
	if fails || foldCount == n {
		wantConsumed = distinct[foldCount-1] + 1
	}
	consumed = 0
	callbackErr := errors.New("callback error")
	got, err := finiteResponses(rows, &consumed).Reduce(n, []NodeResponse[*pb.Int32Value](nil), func(acc []NodeResponse[*pb.Int32Value], r NodeResponse[*pb.Int32Value]) ([]NodeResponse[*pb.Int32Value], error) {
		acc = append(acc, r)
		if len(acc) == failAt {
			return acc, callbackErr
		}
		return acc, nil
	})
	if !slices.Equal(got, want) || consumed != wantConsumed {
		t.Fatalf("Reduce = (%v, %d yields), want (%v, %d)", got, consumed, want, wantConsumed)
	}
	if fails {
		if err != callbackErr {
			t.Fatalf("callback error = %v, want %v", err, callbackErr)
		}
	} else {
		checkTerminalError(t, err, foldCount == n, rows[:wantConsumed])
	}
}
