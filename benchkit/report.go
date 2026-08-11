package benchkit

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"google.golang.org/protobuf/proto"
)

// binaryMagic is the 8-byte sentinel written at the start of every result file:
// "BKRS" (benchkit results) and "v2", followed by a newline and a NUL byte. Its
// job is identification: proto.Unmarshal accepts almost any bytes without error,
// so the sentinel is what lets [DecodeReport] reject a non-benchkit file
// cleanly. Schema evolution is handled by protobuf rules (add fields with new
// numbers), not by this version: bump "v2" only if the on-disk framing itself
// changes. The full contract is in doc/benchkit.html, section 12.
const binaryMagic = "BKRSv2\n\x00"

// WriteReport serializes a labeled report to filename as binary proto: an
// 8-byte magic header followed by the binary-encoded Report message (see
// doc/benchkit.html, section 12). LoadReport reads the file back.
func WriteReport(report *Report, filename string) error {
	reportBytes, err := proto.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal Report: %w", err)
	}
	buf := make([]byte, 0, len(binaryMagic)+len(reportBytes))
	buf = append(buf, binaryMagic...)
	buf = append(buf, reportBytes...)
	return os.WriteFile(filename, buf, 0o644)
}

// LoadReport reads a labeled report from filename written by [WriteReport]. It
// returns an error if the file does not carry the magic header.
func LoadReport(filename string) (*Report, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	report, err := DecodeReport(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filename, err)
	}
	return report, nil
}

// DecodeReport decodes the contents of a result file written by [WriteReport]:
// an 8-byte magic header followed by the binary-encoded Report message. It
// returns an error if the data does not carry the magic header. Use it in place
// of [LoadReport] when the caller reads the file itself, for instance to
// classify read failures of its own (a consumer of a sweep's compact-transfer
// directory expects some result files to be absent).
func DecodeReport(data []byte) (*Report, error) {
	if len(data) < len(binaryMagic) || string(data[:len(binaryMagic)]) != binaryMagic {
		return nil, fmt.Errorf("not a benchkit binary result file")
	}
	var report Report
	if err := proto.Unmarshal(data[len(binaryMagic):], &report); err != nil {
		return nil, fmt.Errorf("unmarshal Report: %w", err)
	}
	return &report, nil
}

// WriteLabeledReport wraps results in a labeled [Report] message and writes
// it to filename via [WriteReport]. The label is typically the run's -label
// flag, falling back to a topology-derived identifier (e.g. -self) when unset.
func WriteLabeledReport(results []*Result, label, filename string) error {
	report := Report_builder{Label: label, Results: results}.Build()
	if err := WriteReport(report, filename); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// CompareWithBaseline loads the report at baselineFile, wraps results in a
// labeled [Report], and writes the side-by-side comparison to w via
// [PrintComparison].
func CompareWithBaseline(baselineFile, label string, results []*Result, w io.Writer) error {
	baseline, err := LoadReport(baselineFile)
	if err != nil {
		return fmt.Errorf("load comparison file: %w", err)
	}
	experiment := Report_builder{Label: label, Results: results}.Build()
	PrintComparison(baseline, experiment, w)
	return nil
}

// PrintComparison writes a side-by-side latency and throughput comparison
// of two reports matched by benchmark name. No statistical tests are
// performed; percentage change is relative to baseline. A benchmark whose
// baseline and experiment configs differ in a field that changes result
// semantics (e.g. quorum size, rate ramp, stream mode) is still compared —
// rejecting outright would break intentional comparisons like dual vs dedup
// stream mode — but the differing fields are printed as a warning
// (see [ConfigDelta]) so the comparison is never silently misleading.
func PrintComparison(baseline, experiment *Report, w io.Writer) {
	fmt.Fprintf(w, "Comparison: %q (baseline) vs %q\n\n",
		baseline.GetLabel(), experiment.GetLabel())

	byName := make(map[string]*Result, len(experiment.GetResults()))
	for _, r := range experiment.GetResults() {
		byName[r.GetConfig().GetName()] = r
	}

	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "Benchmark\tBaseline latency\tExperiment latency\tΔ latency\tBaseline tput\tExperiment tput\tΔ tput")
	for _, b := range baseline.GetResults() {
		e, ok := byName[b.GetConfig().GetName()]
		if !ok {
			continue
		}
		bMean, bStd := b.LatencyMeanAndStdDev()
		eMean, eStd := e.LatencyMeanAndStdDev()
		bTput := b.GetThroughput()
		eTput := e.GetThroughput()

		latDelta := ""
		if bMean > 0 {
			pct := (float64(eMean-bMean) / float64(bMean)) * 100
			latDelta = fmt.Sprintf("%+.1f%%", pct)
		}
		tputDelta := ""
		if bTput > 0 {
			pct := (eTput - bTput) / bTput * 100
			tputDelta = fmt.Sprintf("%+.1f%%", pct)
		}

		fmt.Fprintf(tw, "%s\t%s ± %s\t%s ± %s\t%s\t%.0f ops/s\t%.0f ops/s\t%s\n",
			b.GetConfig().GetName(),
			formatDuration(bMean), formatDuration(bStd),
			formatDuration(eMean), formatDuration(eStd),
			latDelta,
			bTput, eTput,
			tputDelta,
		)
	}
	tw.Flush()

	for _, b := range baseline.GetResults() {
		e, ok := byName[b.GetConfig().GetName()]
		if !ok {
			continue
		}
		if delta := ConfigDelta(b.GetConfig(), e.GetConfig()); len(delta) > 0 {
			fmt.Fprintf(w, "warning: %s: baseline and experiment configs differ: %s\n",
				b.GetConfig().GetName(), strings.Join(delta, ", "))
		}
	}
}

// ConfigDelta returns one "field: baseline vs experiment" string per field in
// a and b that differs and changes what the run measured — everything that
// stamps onto [RunConfig] except the run's own name. An empty result means
// the two configs are semantically comparable. Callers (e.g.
// [PrintComparison]) use this to flag, not reject, comparisons between
// differently configured runs.
func ConfigDelta(a, b *RunConfig) []string {
	var delta []string
	add := func(field string, av, bv any) {
		if av != bv {
			delta = append(delta, fmt.Sprintf("%s: %v vs %v", field, av, bv))
		}
	}
	add("num_nodes", a.GetNumNodes(), b.GetNumNodes())
	add("mode", a.GetMode(), b.GetMode())
	add("duration", time.Duration(a.GetDuration()), time.Duration(b.GetDuration()))
	add("workers", a.GetWorkers(), b.GetWorkers())
	add("payload", a.GetPayload(), b.GetPayload())
	add("rate", a.GetRate(), b.GetRate())
	add("interval", time.Duration(a.GetInterval()), time.Duration(b.GetInterval()))
	add("measurement_mode", a.GetMeasurementMode(), b.GetMeasurementMode())
	add("stats_mode", a.GetStatsMode(), b.GetStatsMode())
	add("stream_mode", a.GetStreamMode(), b.GetStreamMode())
	add("quorum_size", a.GetQuorumSize(), b.GetQuorumSize())
	add("max_async", a.GetMaxAsync(), b.GetMaxAsync())
	add("rate_step", a.GetRateStep(), b.GetRateStep())
	add("rate_step_max", a.GetRateStepMax(), b.GetRateStepMax())
	add("call_timeout", time.Duration(a.GetCallTimeout()), time.Duration(b.GetCallTimeout()))
	add("send_buffer", a.GetSendBuffer(), b.GetSendBuffer())
	add("recv_buffer", a.GetRecvBuffer(), b.GetRecvBuffer())
	return delta
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%.1f ns", float64(d.Nanoseconds()))
	case d < time.Millisecond:
		return fmt.Sprintf("%.1f µs", float64(d.Nanoseconds())/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.1f ms", float64(d.Nanoseconds())/1e6)
	default:
		return fmt.Sprintf("%.1f s", d.Seconds())
	}
}
