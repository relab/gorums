package main

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/relab/gorums/benchkit"
)

// qRun builds a per-rep plot record for a fixed Q/N3/W?/P0 configuration; the
// same latency value stands in for every percentile.
func qRun(workers int, mode, status string, thr, lat float64) plotRunRecord {
	return plotRunRecord{
		Dimensions: benchkit.Dimensions{
			Benchmark: "Q", Nodes: 3, Workers: workers, StreamMode: mode,
		},
		status: status, throughput: thr,
		allocsPerOp: 1, memPerOp: 100,
		p50US: new(lat), p95US: new(lat), p99US: new(lat), meanUS: new(lat),
	}
}

// TestAggregateRepsSeparatesBufferSizes verifies that runs differing only by a
// buffer capacity aggregate into separate rows, and that the capacities reach
// the CSV. Folding them together would report one blended row per
// configuration and silently average across the setting under test.
func TestAggregateRepsSeparatesBufferSizes(t *testing.T) {
	bufRun := func(sendBuffer, recvBuffer int, thr float64) plotRunRecord {
		r := qRun(1, "dual", runStatusSucceeded, thr, 10.0)
		r.SendBuffer, r.RecvBuffer = sendBuffer, recvBuffer
		return r
	}
	runs := []plotRunRecord{
		bufRun(64, 0, 100), bufRun(64, 0, 120),
		bufRun(4096, 0, 300),
		bufRun(64, 16, 200),
	}
	agg := aggregateReps(runs, false)
	if len(agg) != 3 {
		t.Fatalf("aggregated to %d rows, want 3 (one per distinct buffer pair)", len(agg))
	}
	got := map[[2]int]float64{}
	for _, r := range agg {
		got[[2]int{r.SendBuffer, r.RecvBuffer}] = r.throughput.mean
	}
	want := map[[2]int]float64{{64, 0}: 110, {4096, 0}: 300, {64, 16}: 200}
	for k, w := range want {
		if g, ok := got[k]; !ok {
			t.Errorf("missing row for send=%d recv=%d", k[0], k[1])
		} else if math.Abs(g-w) > 1e-9 {
			t.Errorf("send=%d recv=%d throughput = %v, want %v", k[0], k[1], g, w)
		}
	}

	path := filepath.Join(t.TempDir(), "agg.csv")
	if err := writeAggRunsCSV(path, agg); err != nil {
		t.Fatalf("writeAggRunsCSV: %v", err)
	}
	recs, err := csv.NewReader(mustOpen(t, path)).ReadAll()
	if err != nil {
		t.Fatalf("read agg.csv: %v", err)
	}
	sendCol := slices.Index(recs[0], "send_buffer")
	recvCol := slices.Index(recs[0], "recv_buffer")
	if sendCol < 0 || recvCol < 0 {
		t.Fatalf("agg.csv header lacks buffer columns: %v", recs[0])
	}
	seen := map[[2]string]bool{}
	for _, rec := range recs[1:] {
		seen[[2]string{rec[sendCol], rec[recvCol]}] = true
	}
	if len(seen) != 3 {
		t.Errorf("agg.csv has %d distinct buffer pairs, want 3", len(seen))
	}
}

// mustOpen opens path for reading and closes it when the test ends.
func mustOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func TestAggregateReps(t *testing.T) {
	runs := []plotRunRecord{
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dual", runStatusSucceeded, 200, 20.0),
		qRun(2, "dual", runStatusDegraded, 999, 999.0),
		qRun(4, "dual", runStatusSucceeded, 50, 5.0),
	}

	t.Run("ExcludeDegraded", func(t *testing.T) {
		got := aggregateReps(runs, false)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		// Sorted: (W2) before (W4).
		w2 := got[0]
		if w2.Workers != 2 {
			t.Fatalf("got[0].workers = %d, want 2", w2.Workers)
		}
		if w2.reps != 2 || w2.repsDegraded != 1 {
			t.Errorf("reps=%d repsDegraded=%d, want 2 and 1", w2.reps, w2.repsDegraded)
		}
		// mean(100,200)=150; sample sd=sqrt(5000)=70.7107; ci95=t(df=1)*sd/sqrt(2).
		wantSD := math.Sqrt(5000)
		wantCI := 12.706 * wantSD / math.Sqrt(2)
		assertStat(t, "throughput", w2.throughput, aggStat{mean: 150, sd: wantSD, ci95: wantCI, n: 2})
		// p50 mean(10,20)=15.
		assertStat(t, "p50US", w2.p50US, aggStat{mean: 15, sd: math.Sqrt(50), ci95: 12.706 * math.Sqrt(50) / math.Sqrt(2), n: 2})

		w4 := got[1]
		if w4.reps != 1 || w4.repsDegraded != 0 {
			t.Errorf("W4 reps=%d repsDegraded=%d, want 1 and 0", w4.reps, w4.repsDegraded)
		}
		assertStat(t, "throughput", w4.throughput, aggStat{mean: 50, sd: 0, ci95: 0, n: 1})
	})

	t.Run("IncludeDegraded", func(t *testing.T) {
		got := aggregateReps(runs, true)
		w2 := got[0]
		if w2.reps != 3 || w2.repsDegraded != 1 {
			t.Errorf("reps=%d repsDegraded=%d, want 3 and 1", w2.reps, w2.repsDegraded)
		}
		if math.Abs(w2.throughput.mean-433) > 0.5 {
			t.Errorf("throughput mean = %g, want ~433", w2.throughput.mean)
		}
	})
}

func TestWriteAggRunsCSV(t *testing.T) {
	rows := aggregateReps([]plotRunRecord{
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dual", runStatusSucceeded, 200, 20.0),
	}, false)
	path := filepath.Join(t.TempDir(), "agg.csv")
	if err := writeAggRunsCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("rows = %d (incl header), want 2", len(recs))
	}
	header := recs[0]
	col := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		t.Fatalf("column %q not in header %v", name, header)
		return -1
	}
	row := recs[1]
	if row[col("benchmark")] != "Q" || row[col("workers")] != "2" {
		t.Errorf("benchmark/workers = %q/%q", row[col("benchmark")], row[col("workers")])
	}
	if row[col("throughput")] != "150" {
		t.Errorf("throughput = %q, want 150", row[col("throughput")])
	}
	if row[col("reps")] != "2" {
		t.Errorf("reps = %q, want 2", row[col("reps")])
	}
	// ms mirror of the us column: p50_us=15 -> p50_ms=0.015.
	if row[col("p50_ms")] != "0.015" {
		t.Errorf("p50_ms = %q, want 0.015", row[col("p50_ms")])
	}
}

// TestRatioStat verifies ratioStat's propagated-uncertainty formula
// (sd_r = |r|*sqrt((sd_x/x)^2 + (sd_y/y)^2)) and its three "not comparable"
// cases: an absent metric on either side, and a zero baseline mean.
func TestRatioStat(t *testing.T) {
	t.Run("PropagatesRelativeUncertainty", func(t *testing.T) {
		x := aggStat{mean: 200, sd: 20, n: 3}
		y := aggStat{mean: 100, sd: 10, n: 5}
		got, ok := ratioStat(x, y)
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if math.Abs(got.mean-2.0) > 1e-9 {
			t.Errorf("mean = %v, want 2.0", got.mean)
		}
		// rel = (20/200)^2 + (10/100)^2 = 0.01 + 0.01 = 0.02; sd = 2*sqrt(0.02).
		wantSD := 2.0 * math.Sqrt(0.02)
		if math.Abs(got.sd-wantSD) > 1e-9 {
			t.Errorf("sd = %v, want %v", got.sd, wantSD)
		}
		if got.n != 3 {
			t.Errorf("n = %d, want min(3, 5) = 3", got.n)
		}
	})

	t.Run("ZeroBaselineMeanIsNotComparable", func(t *testing.T) {
		_, ok := ratioStat(aggStat{mean: 100, n: 3}, aggStat{mean: 0, n: 3})
		if ok {
			t.Error("ok = true with a zero baseline mean, want false")
		}
	})

	t.Run("AbsentXIsNotComparable", func(t *testing.T) {
		_, ok := ratioStat(aggStat{n: 0}, aggStat{mean: 100, n: 3})
		if ok {
			t.Error("ok = true with x.n == 0, want false")
		}
	})

	t.Run("AbsentYIsNotComparable", func(t *testing.T) {
		_, ok := ratioStat(aggStat{mean: 100, n: 3}, aggStat{n: 0})
		if ok {
			t.Error("ok = true with y.n == 0, want false")
		}
	})

	t.Run("ZeroXMeanSkipsItsOwnRelativeTerm", func(t *testing.T) {
		// x.mean == 0 (but n > 0, unlike the AbsentX case) must not divide by
		// zero computing its own relative term; only y's term contributes.
		x := aggStat{mean: 0, sd: 5, n: 3}
		y := aggStat{mean: 100, sd: 10, n: 3}
		got, ok := ratioStat(x, y)
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if got.mean != 0 {
			t.Errorf("mean = %v, want 0", got.mean)
		}
		if got.sd != 0 {
			t.Errorf("sd = %v, want 0 (|r|=0 zeroes the propagated sd regardless of rel)", got.sd)
		}
	})
}

func TestPivotComparison(t *testing.T) {
	agg := aggregateReps([]plotRunRecord{
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dedup", runStatusSucceeded, 150, 8.0),
		qRun(2, "dedup", runStatusSucceeded, 150, 8.0),
	}, false)

	t.Run("DefaultBaselineDual", func(t *testing.T) {
		cmp := pivotComparison(agg, "")
		if len(cmp) != 1 {
			t.Fatalf("len = %d, want 1", len(cmp))
		}
		c := cmp[0]
		if c.baseline != "dual" {
			t.Errorf("baseline = %q, want dual", c.baseline)
		}
		if len(c.perMode) != 2 {
			t.Fatalf("perMode has %d modes, want 2", len(c.perMode))
		}
		// dedup/dual throughput ratio = 150/100 = 1.5.
		ratio, ok := ratioStat(c.perMode["dedup"].throughput, c.perMode["dual"].throughput)
		if !ok || math.Abs(ratio.mean-1.5) > 1e-9 {
			t.Errorf("throughput ratio = %+v ok=%v, want mean 1.5", ratio, ok)
		}
	})

	t.Run("SingleModeYieldsNil", func(t *testing.T) {
		one := aggregateReps([]plotRunRecord{
			qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		}, false)
		if got := pivotComparison(one, ""); got != nil {
			t.Errorf("pivotComparison with one mode = %v, want nil", got)
		}
	})
}

func TestPivotComparisonRetainsBufferDimensions(t *testing.T) {
	var runs []plotRunRecord
	for _, send := range []int{64, 256} {
		for _, mode := range []string{"dual", "dedup"} {
			r := qRun(2, mode, runStatusSucceeded, float64(send), 10)
			r.SendBuffer = send
			runs = append(runs, r)
		}
	}
	rows := pivotComparison(aggregateReps(runs, false), "dual")
	if len(rows) != 2 {
		t.Fatalf("comparison rows = %d, want 2 buffer configurations", len(rows))
	}
	if rows[0].SendBuffer == rows[1].SendBuffer {
		t.Fatalf("buffer configurations merged: %+v", rows)
	}
	for _, row := range rows {
		if len(row.perMode) != 2 {
			t.Errorf("send_buffer=%d has %d modes, want 2", row.SendBuffer, len(row.perMode))
		}
	}
}

func TestWriteComparisonCSV(t *testing.T) {
	agg := aggregateReps([]plotRunRecord{
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dual", runStatusSucceeded, 100, 10.0),
		qRun(2, "dedup", runStatusSucceeded, 150, 8.0),
		qRun(2, "dedup", runStatusSucceeded, 150, 8.0),
	}, false)
	rows := pivotComparison(agg, "")
	path := filepath.Join(t.TempDir(), "comparison.csv")
	if err := writeComparisonCSV(path, rows); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("rows = %d (incl header), want 2", len(recs))
	}
	header, row := recs[0], recs[1]
	col := func(name string) int {
		i := slices.Index(header, name)
		if i < 0 {
			t.Fatalf("column %q not in header %v", name, header)
		}
		return i
	}
	if row[col("throughput_dual")] != "100" || row[col("throughput_dedup")] != "150" {
		t.Errorf("throughput dual/dedup = %q/%q", row[col("throughput_dual")], row[col("throughput_dedup")])
	}
	if row[col("throughput_ratio")] != "1.5" {
		t.Errorf("throughput_ratio = %q, want 1.5", row[col("throughput_ratio")])
	}
	if row[col("modes")] != "dedup|dual" {
		t.Errorf("modes = %q, want dedup|dual", row[col("modes")])
	}
}

// TestTCritical95 verifies the two-tailed 95% Student's t critical value at
// typical sweep repetition counts (small df, where it diverges sharply from
// the normal distribution) and beyond the exact table, where the approximation
// must converge smoothly toward the normal critical value.
func TestTCritical95(t *testing.T) {
	tests := []struct {
		df   int
		want float64
	}{
		{1, 12.706}, // 2 reps
		{2, 4.303},  // 3 reps
		{4, 2.776},  // 5 reps
		{30, 2.042},
		{31, 2.0395},
		{40, 2.0211},
		{100, 1.9840},
		{1000, 1.9623},
	}
	for _, tt := range tests {
		if got := tCritical95(tt.df); math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("tCritical95(%d) = %v, want %v", tt.df, got, tt.want)
		}
	}
}

func assertStat(t *testing.T, name string, got, want aggStat) {
	t.Helper()
	const eps = 1e-6
	if math.Abs(got.mean-want.mean) > eps || math.Abs(got.sd-want.sd) > eps ||
		math.Abs(got.ci95-want.ci95) > eps || got.n != want.n {
		t.Errorf("%s = %+v, want %+v", name, got, want)
	}
}

// TestRepOutliers verifies the report's cross-repetition check: a repetition far
// from its configuration's median is named in either direction — the run-over
// case was 14x above it, which a one-sided check misses — while healthy
// repetition scatter, configurations with too few repetitions, and repetitions
// already flagged degraded are left alone.
func TestRepOutliers(t *testing.T) {
	dims := func(workers int) benchkit.Dimensions {
		return benchkit.Dimensions{Benchmark: "Q", Nodes: 9, Workers: workers, StreamMode: "dedup"}
	}
	runs := []plotRunRecord{
		// Healthy scatter around 5000 across four reps.
		{Dimensions: dims(8), base: "r1", status: runStatusSucceeded, throughput: 4800},
		{Dimensions: dims(8), base: "r2", status: runStatusSucceeded, throughput: 5000},
		{Dimensions: dims(8), base: "r3", status: runStatusSucceeded, throughput: 5200},
		// The run-over case: one rep far above its siblings, plus one far below.
		{Dimensions: dims(16), base: "s1", status: runStatusSucceeded, throughput: 5000},
		{Dimensions: dims(16), base: "s2", status: runStatusSucceeded, throughput: 5100},
		{Dimensions: dims(16), base: "s3", status: runStatusSucceeded, throughput: 710000},
		{Dimensions: dims(16), base: "s4", status: runStatusSucceeded, throughput: 100},
		// Already reported as degraded; not named again.
		{Dimensions: dims(16), base: "s5", status: runStatusDegraded, throughput: 12},
		// Two reps only: neither is the outlier.
		{Dimensions: dims(32), base: "t1", status: runStatusSucceeded, throughput: 5000},
		{Dimensions: dims(32), base: "t2", status: runStatusSucceeded, throughput: 50000},
	}
	notes := repOutliers(runs, repOutlierSpread)
	if len(notes) != 2 {
		t.Fatalf("notes = %v, want 2 (s3 above, s4 below)", notes)
	}
	for i, want := range []string{"run s3: 710000 ops/s is 140.59x", "run s4: 100 ops/s is 0.02x"} {
		if !strings.HasPrefix(notes[i], want) {
			t.Errorf("notes[%d] = %q, want prefix %q", i, notes[i], want)
		}
	}
}
