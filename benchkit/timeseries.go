package benchkit

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// This file renders the time-series event stream (Result.events) to CSV so a
// plotting front end can consume it: WriteTimeSeriesCSVs turns a run's
// per-node event streams into per-benchmark throughput, latency, and
// saturation CSVs.

// Plotter consumes events from a single benchmark's event stream. Add
// dispatches one event from the named node (the Report label); Render writes
// the accumulated data to w.
type Plotter interface {
	Add(node string, e *Event)
	Render(w io.Writer) error
}

// EventReader fans one event stream to every registered Plotter, dropping the
// startup transient (throughput and latency intervals) before trimNs; phase
// markers always pass through so the run's lifecycle structure is preserved.
type EventReader struct {
	plotters []Plotter
	trimNs   int64
}

// NewEventReader returns an EventReader that dispatches each event to every
// registered plotter, dropping interval events recorded before trimNs (0 keeps
// the whole run).
func NewEventReader(trimNs int64, plotters ...Plotter) *EventReader {
	return &EventReader{plotters: plotters, trimNs: trimNs}
}

// Read iterates one node's events and dispatches each to every registered
// plotter, tagged with the node identity so multi-node CSV rows stay
// distinguishable.
func (r *EventReader) Read(node string, events []*Event) {
	for _, ev := range events {
		if r.trimNs > 0 && ev.GetPhase() == nil && ev.GetOffset() < r.trimNs {
			continue
		}
		for _, p := range r.plotters {
			p.Add(node, ev)
		}
	}
}

// throughputRow is one ThroughputInterval observation with its offset and any
// active phase annotation.
type throughputRow struct {
	node    string  // node identity (Report label) the interval came from
	offsetS float64 // seconds since START
	ops     uint64  // operations in this interval
	dur     int64   // nanoseconds; actual interval duration
	phase   string  // annotation: "START", "RATE_STEP", etc.; empty otherwise
}

// ThroughputTimePlotter collects ThroughputInterval and PhaseMarker events and
// renders columns: offset_s, throughput_ops_s, phase, node. The zero value is
// ready to use.
type ThroughputTimePlotter struct {
	rows         []throughputRow
	pendingPhase string
}

// Add processes one event from node.
func (p *ThroughputTimePlotter) Add(node string, e *Event) {
	if ph := e.GetPhase(); ph != nil {
		p.pendingPhase = ph.GetPhase().String()
		return
	}
	if tp := e.GetThroughput(); tp != nil {
		p.rows = append(p.rows, throughputRow{
			node:    node,
			offsetS: float64(e.GetOffset()) / 1e9,
			ops:     tp.GetOps(),
			dur:     tp.GetDuration(),
			phase:   p.pendingPhase,
		})
		p.pendingPhase = ""
	}
}

// Render writes CSV to w. node and phase come from report labels and event
// data, not fixed enums, so data rows go through encoding/csv: an embedded
// comma, quote, or newline would otherwise corrupt the file.
func (p *ThroughputTimePlotter) Render(w io.Writer) error {
	return writeCSV(w,
		[]string{"offset_s", "throughput_ops_s", "phase", "node"},
		p.rows, func(r throughputRow) []string {
			thr := 0.0
			if r.dur > 0 {
				thr = float64(r.ops) / (float64(r.dur) / 1e9)
			}
			return []string{
				strconv.FormatFloat(r.offsetS, 'f', 6, 64),
				strconv.FormatFloat(thr, 'f', 3, 64),
				r.phase,
				r.node,
			}
		})
}

// latencyRow is one LatencyInterval observation.
type latencyRow struct {
	node     string
	offsetS  float64
	meanNs   float64
	stddevNs float64
	count    uint64
	phase    string
}

// LatencyTimePlotter collects LatencyInterval and PhaseMarker events and
// renders columns: offset_s, mean_ns, stddev_ns, count, phase, node. The zero
// value is ready to use.
type LatencyTimePlotter struct {
	rows         []latencyRow
	pendingPhase string
}

// Add processes one event from node.
func (p *LatencyTimePlotter) Add(node string, e *Event) {
	if ph := e.GetPhase(); ph != nil {
		p.pendingPhase = ph.GetPhase().String()
		return
	}
	if lat := e.GetLatency(); lat != nil {
		p.rows = append(p.rows, latencyRow{
			node:     node,
			offsetS:  float64(e.GetOffset()) / 1e9,
			meanNs:   lat.GetMean(),
			stddevNs: lat.GetStddev(),
			count:    lat.GetCount(),
			phase:    p.pendingPhase,
		})
		p.pendingPhase = ""
	}
}

// Render writes CSV to w (see [ThroughputTimePlotter.Render]).
func (p *LatencyTimePlotter) Render(w io.Writer) error {
	return writeCSV(w,
		[]string{"offset_s", "mean_ns", "stddev_ns", "count", "phase", "node"},
		p.rows, func(r latencyRow) []string {
			return []string{
				strconv.FormatFloat(r.offsetS, 'f', 6, 64),
				strconv.FormatFloat(r.meanNs, 'f', 3, 64),
				strconv.FormatFloat(r.stddevNs, 'f', 3, 64),
				strconv.FormatUint(r.count, 10),
				r.phase,
				r.node,
			}
		})
}

// rateLevel accumulates throughput and latency samples for one rate-ramp step
// of one node.
type rateLevel struct {
	node         string
	offeredRate  int64
	totalOps     uint64
	totalDurNs   int64
	latencySum   float64
	latencyCount uint64
}

// SaturationCurvePlotter builds a saturation curve (offered rate vs achieved
// throughput) from PhaseMarker(RATE_STEP) and ThroughputInterval events. A
// single-rate run with no RATE_STEP events is treated as one level. Each node's
// START opens that node's first level, so multi-node input yields one set of
// levels per node. Render columns: offered_rate, throughput_ops_s,
// mean_latency_ns, node. The zero value is ready to use.
type SaturationCurvePlotter struct {
	levels    []*rateLevel
	current   *rateLevel
	inMeasure bool // true after START
}

// Add processes one event from node.
func (p *SaturationCurvePlotter) Add(node string, e *Event) {
	if ph := e.GetPhase(); ph != nil {
		switch ph.GetPhase() {
		case PhaseMarker_START:
			// Measurement begins at t=0; open the first level at the initial rate.
			p.inMeasure = true
			p.current = &rateLevel{node: node, offeredRate: ph.GetRate()}
			p.levels = append(p.levels, p.current)
		case PhaseMarker_RATE_STEP:
			// Transition to a new rate level.
			p.current = &rateLevel{node: node, offeredRate: ph.GetRate()}
			p.levels = append(p.levels, p.current)
		}
		return
	}
	if !p.inMeasure || p.current == nil {
		return
	}
	if tp := e.GetThroughput(); tp != nil {
		p.current.totalOps += tp.GetOps()
		p.current.totalDurNs += tp.GetDuration()
		return
	}
	if lat := e.GetLatency(); lat != nil {
		p.current.latencySum += lat.GetMean() * float64(lat.GetCount())
		p.current.latencyCount += lat.GetCount()
	}
}

// Render writes CSV to w (see [ThroughputTimePlotter.Render]).
func (p *SaturationCurvePlotter) Render(w io.Writer) error {
	return writeCSV(w,
		[]string{"offered_rate", "throughput_ops_s", "mean_latency_ns", "node"},
		p.levels, func(lv *rateLevel) []string {
			thr := 0.0
			if lv.totalDurNs > 0 {
				thr = float64(lv.totalOps) / (float64(lv.totalDurNs) / 1e9)
			}
			meanLat := 0.0
			if lv.latencyCount > 0 {
				meanLat = lv.latencySum / float64(lv.latencyCount)
			}
			return []string{
				strconv.FormatInt(lv.offeredRate, 10),
				strconv.FormatFloat(thr, 'f', 3, 64),
				strconv.FormatFloat(meanLat, 'f', 3, 64),
				lv.node,
			}
		})
}

// TimeSeriesNode is one node's event stream, labeled by the node it came from
// (the Report label, or whatever identity the caller assigns).
type TimeSeriesNode struct {
	Node   string
	Events []*Event
}

// TimeSeriesGroup is one benchmark's per-node event streams, the unit the
// time-series renderer draws a figure from.
type TimeSeriesGroup struct {
	Benchmark string
	Nodes     []TimeSeriesNode
}

// WriteTimeSeriesCSVs renders each group's throughput-over-time,
// latency-over-time, and saturation-curve CSV into outDir as
// <benchmark>_throughput.csv, <benchmark>_latency.csv, and
// <benchmark>_saturation.csv, and returns the benchmarks that had data, in the
// order given, so the caller can plan one figure per name. Multi-node rows stay
// distinguishable via the node column. Interval events before trim are dropped,
// consistent with the read-time trim [Summarize] applies. The output directory
// is created if it does not exist.
//
// A group whose streams hold no throughput or latency interval — a run measured
// with interval reporting off, or one whose every interval fell before trim — is
// skipped entirely: it writes no CSV and is absent from the returned names, so
// no figure is planned for it. Header-only CSVs would instead leave the figure's
// node list empty, which a report cannot render.
func WriteTimeSeriesCSVs(outDir string, groups []TimeSeriesGroup, trim time.Duration) ([]string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	var available []string
	for _, group := range groups {
		tp := &ThroughputTimePlotter{}
		lp := &LatencyTimePlotter{}
		sc := &SaturationCurvePlotter{}
		reader := NewEventReader(trim.Nanoseconds(), tp, lp, sc)
		for _, node := range group.Nodes {
			reader.Read(node.Node, node.Events)
		}
		if len(tp.rows) == 0 && len(lp.rows) == 0 {
			continue
		}
		for _, task := range []struct {
			plotter  Plotter
			filename string
		}{
			{tp, group.Benchmark + "_throughput.csv"},
			{lp, group.Benchmark + "_latency.csv"},
			{sc, group.Benchmark + "_saturation.csv"},
		} {
			if err := renderTimeSeries(task.plotter, filepath.Join(outDir, task.filename)); err != nil {
				return nil, err
			}
		}
		available = append(available, group.Benchmark)
	}
	return available, nil
}

// renderTimeSeries renders one plotter's output to the file at path.
func renderTimeSeries(p Plotter, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	err = p.Render(f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("render %s: %w", filepath.Base(path), err)
	}
	return nil
}

// writeCSV writes a header and one record per row to w.
func writeCSV[T any](w io.Writer, header []string, rows []T, fields func(T) []string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := cw.Write(fields(row)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
