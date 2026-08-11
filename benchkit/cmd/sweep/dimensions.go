package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/relab/gorums/benchkit"
)

func compareDimensions(a, b benchkit.Dimensions) int {
	return cmp.Or(
		cmp.Compare(a.Benchmark, b.Benchmark),
		cmp.Compare(a.Nodes, b.Nodes),
		cmp.Compare(a.Workers, b.Workers),
		cmp.Compare(a.Payload, b.Payload),
		cmp.Compare(a.Rate, b.Rate),
		cmp.Compare(a.SendBuffer, b.SendBuffer),
		cmp.Compare(a.RecvBuffer, b.RecvBuffer),
		cmp.Compare(a.StreamMode, b.StreamMode),
	)
}

func comparisonDimensions(d benchkit.Dimensions) benchkit.Dimensions {
	d.StreamMode = ""
	return d
}

// loadScaleDimensions extracts the dimensions that drive a throughput-latency
// curve's scale (see tlIdent): payload and rate, plus the buffer capacities,
// which can shift peak latency by an order of magnitude (bufferbloat) without
// otherwise identifying the load. The dimensions in loads are cleared: a curve
// traces along them, so points differing only there belong to one curve.
func loadScaleDimensions(d benchkit.Dimensions, loads []string) benchkit.Dimensions {
	scale := benchkit.Dimensions{
		Payload: d.Payload, Rate: d.Rate,
		SendBuffer: d.SendBuffer, RecvBuffer: d.RecvBuffer,
	}
	if slices.Contains(loads, "rate") {
		scale.Rate = 0
	}
	return scale
}

func nodeHealthDimensions(d benchkit.Dimensions) benchkit.Dimensions {
	return benchkit.Dimensions{
		Benchmark: d.Benchmark, Nodes: d.Nodes, StreamMode: d.StreamMode,
	}
}

// dimensionSpec describes one sweep dimension: its CSV column name, the
// human-readable axis label a figure gives it, the short tag a compact
// configuration label uses (empty for the dimensions whose value speaks for
// itself), and how to read its value from a record.
type dimensionSpec struct {
	name  string
	label string
	tag   string
	value func(benchkit.Dimensions) string
}

var dimensionSpecs = []dimensionSpec{
	{"benchmark", "Benchmark", "", func(d benchkit.Dimensions) string { return d.Benchmark }},
	{"nodes", "Nodes (N)", "N", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Nodes) }},
	{"workers", "Workers", "W", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Workers) }},
	{"payload", "Payload (bytes)", "P", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Payload) }},
	{"rate", "Offered rate (ops/s per node)", "R", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Rate) }},
	{"send_buffer", "Send queue capacity (requests)", "SB", func(d benchkit.Dimensions) string { return strconv.Itoa(d.SendBuffer) }},
	{"recv_buffer", "Receive queue capacity (messages)", "RB", func(d benchkit.Dimensions) string { return strconv.Itoa(d.RecvBuffer) }},
	{"stream_mode", "Stream mode", "", func(d benchkit.Dimensions) string { return d.StreamMode }},
}

// varyingDimensions returns the dimensions whose value differs across the given
// configurations. It is what a compact label must name to identify one of them:
// a label repeating what every configuration shares says nothing about which
// one it labels, and what they all share belongs in the report header instead.
func varyingDimensions(configs []benchkit.Dimensions) map[string]bool {
	varying := make(map[string]bool, len(dimensionSpecs))
	for _, dim := range dimensionSpecs {
		for _, config := range configs {
			if dim.value(config) != dim.value(configs[0]) {
				varying[dim.name] = true
				break
			}
		}
	}
	return varying
}

// configLabel names one configuration compactly, listing only the dimensions in
// varying: "N15 P16384 R1000 dedup". Tagged dimensions render as tag+value, the
// benchmark and stream mode as their bare value. An unset numeric dimension
// (value 0, the marker for "not swept") is left out. The label is empty when
// nothing varies, which leaves the caller's own heading to identify the run.
func configLabel(dims benchkit.Dimensions, varying map[string]bool) string {
	var parts []string
	for _, dim := range dimensionSpecs {
		if !varying[dim.name] {
			continue
		}
		value := dim.value(dims)
		if value == "" || value == "0" {
			continue
		}
		parts = append(parts, dim.tag+value)
	}
	return strings.Join(parts, " ")
}

// dimensionValue returns one dimension's value as a string, or "" when name
// does not name a dimension — including the empty name a figure uses for an
// absent facet.
func dimensionValue(dims benchkit.Dimensions, name string) string {
	dim, ok := findDimension(name)
	if !ok {
		return ""
	}
	return dim.value(dims)
}

func findDimension(name string) (dimensionSpec, bool) {
	for _, dim := range dimensionSpecs {
		if dim.name == name {
			return dim, true
		}
	}
	return dimensionSpec{}, false
}

func dimensionColumns(omit ...string) []string {
	skip := make(map[string]bool, len(omit))
	for _, name := range omit {
		skip[name] = true
	}
	var columns []string
	for _, dim := range dimensionSpecs {
		if !skip[dim.name] {
			columns = append(columns, dim.name)
		}
	}
	return columns
}

func dimensionValues(dims benchkit.Dimensions, omit ...string) []string {
	skip := make(map[string]bool, len(omit))
	for _, name := range omit {
		skip[name] = true
	}
	var values []string
	for _, dim := range dimensionSpecs {
		if !skip[dim.name] {
			values = append(values, dim.value(dims))
		}
	}
	return values
}
