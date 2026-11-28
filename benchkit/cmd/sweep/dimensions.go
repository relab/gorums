package main

import (
	"cmp"
	"regexp"
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
		compareCustom(a.Custom, b.Custom),
	)
}

// compareCustom orders custom dimension values name by name, in name order,
// with [compareDimensionValues].
func compareCustom(a, b benchkit.CustomDimensions) int {
	if a == b {
		return 0
	}
	names := slices.Concat(a.Names(), b.Names())
	slices.Sort(names)
	for _, name := range slices.Compact(names) {
		if c := compareDimensionValues(a.Get(name), b.Get(name)); c != 0 {
			return c
		}
	}
	return 0
}

// numericValuePattern matches a dimension value that sorts as a number. The
// Typst helper library uses the same pattern, so a value is numeric in a report
// figure exactly when it is numeric here.
var numericValuePattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// compareDimensionValues orders dimension values: the empty value (absent)
// first, then the numeric values by value, then the others as strings.
func compareDimensionValues(a, b string) int {
	rank := func(v string) int {
		switch {
		case v == "":
			return 0
		case numericValuePattern.MatchString(v):
			return 1
		default:
			return 2
		}
	}
	ra, rb := rank(a), rank(b)
	if ra != rb {
		return cmp.Compare(ra, rb)
	}
	if ra == 1 {
		fa, _ := strconv.ParseFloat(a, 64)
		fb, _ := strconv.ParseFloat(b, 64)
		if c := cmp.Compare(fa, fb); c != 0 {
			return c
		}
	}
	return strings.Compare(a, b)
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

// nodeHealthDimensions extracts the dimensions that identify a node-health
// configuration: the benchmark, the node count, the stream mode, and the custom
// dimensions. The load dimensions are cleared, so the relative throughput of a
// host summarizes every load point of the configuration.
func nodeHealthDimensions(d benchkit.Dimensions) benchkit.Dimensions {
	return benchkit.Dimensions{
		Benchmark: d.Benchmark, Nodes: d.Nodes, StreamMode: d.StreamMode, Custom: d.Custom,
	}
}

// dimensionSpec describes one sweep dimension: its CSV column name, the
// human-readable axis label a figure gives it, the short tag a compact
// configuration label uses (empty for the dimensions whose value speaks for
// itself), and how to read its value from a record. custom marks a dimension
// that a -dim flag declares; its value is a string, and "0" is a real value
// rather than the unset marker of a built-in numeric dimension.
type dimensionSpec struct {
	name   string
	label  string
	tag    string
	value  func(benchkit.Dimensions) string
	custom bool
}

// customDimensionSpec returns the spec of the custom dimension name: its label
// is the name, and a compact label tags its value as name=value.
func customDimensionSpec(name string) dimensionSpec {
	return dimensionSpec{
		name:   name,
		label:  name,
		tag:    name + "=",
		value:  func(d benchkit.Dimensions) string { return d.Custom.Get(name) },
		custom: true,
	}
}

// unset reports whether value marks the dimension as unconfigured: an
// absent custom value, or a zero built-in value.
func (dim dimensionSpec) unset(value string) bool {
	return value == "" || (!dim.custom && value == "0")
}

// builtinDimensions are the dimensions every configuration has, in display order.
var builtinDimensions = []dimensionSpec{
	{"benchmark", "Benchmark", "", func(d benchkit.Dimensions) string { return d.Benchmark }, false},
	{"nodes", "Nodes (N)", "N", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Nodes) }, false},
	{"workers", "Workers", "W", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Workers) }, false},
	{"payload", "Payload (bytes)", "P", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Payload) }, false},
	{"rate", "Offered rate (ops/s per node)", "R", func(d benchkit.Dimensions) string { return strconv.Itoa(d.Rate) }, false},
	{"send_buffer", "Send queue capacity (requests)", "SB", func(d benchkit.Dimensions) string { return strconv.Itoa(d.SendBuffer) }, false},
	{"recv_buffer", "Receive queue capacity (messages)", "RB", func(d benchkit.Dimensions) string { return strconv.Itoa(d.RecvBuffer) }, false},
	{"stream_mode", "Stream mode", "", func(d benchkit.Dimensions) string { return d.StreamMode }, false},
}

// dimensionSchema is the ordered list of dimensions that a set of
// configurations holds: the built-in dimensions, then the custom dimensions of
// any configuration, sorted by name. It fixes the dimension columns of every
// CSV file and the axes a figure can vary along.
type dimensionSchema []dimensionSpec

// newDimensionSchema returns the schema of configs.
func newDimensionSchema(configs []benchkit.Dimensions) dimensionSchema {
	var names []string
	for _, config := range configs {
		names = append(names, config.Custom.Names()...)
	}
	slices.Sort(names)
	schema := slices.Clone(dimensionSchema(builtinDimensions))
	for _, name := range slices.Compact(names) {
		schema = append(schema, customDimensionSpec(name))
	}
	return schema
}

// schemaOf returns the schema of rows, reading each row's configuration with dims.
func schemaOf[T any](rows []T, dims func(T) benchkit.Dimensions) dimensionSchema {
	configs := make([]benchkit.Dimensions, len(rows))
	for i, row := range rows {
		configs[i] = dims(row)
	}
	return newDimensionSchema(configs)
}

// columns returns the column names of the schema, without the names in omit.
func (s dimensionSchema) columns(omit ...string) []string {
	var columns []string
	for _, dim := range s {
		if !slices.Contains(omit, dim.name) {
			columns = append(columns, dim.name)
		}
	}
	return columns
}

// values returns the values of dims in schema order, without the dimensions
// in omit.
func (s dimensionSchema) values(dims benchkit.Dimensions, omit ...string) []string {
	var values []string
	for _, dim := range s {
		if !slices.Contains(omit, dim.name) {
			values = append(values, dim.value(dims))
		}
	}
	return values
}

// axes returns the names of the dimensions a figure can vary along or facet
// by: every dimension other than the benchmark and the stream mode, which
// identify a series instead.
func (s dimensionSchema) axes() []string {
	return s.columns("benchmark", "stream_mode")
}

// varyingDimensions returns the dimensions whose value differs across the given
// configurations. It is what a compact label must name to identify one of them:
// a label repeating what every configuration shares says nothing about which
// one it labels, and what they all share belongs in the report header instead.
func varyingDimensions(configs []benchkit.Dimensions) map[string]bool {
	schema := newDimensionSchema(configs)
	varying := make(map[string]bool, len(schema))
	for _, dim := range schema {
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
// varying: "N15 P16384 R1000 dedup d=20". Tagged dimensions render as tag+value,
// the benchmark and stream mode as their bare value, and a custom dimension as
// name=value. An unset dimension (a built-in value 0, the marker for "not
// swept", or an absent custom value) is left out. The label is empty when
// nothing varies, which leaves the caller's own heading to identify the run.
func configLabel(dims benchkit.Dimensions, varying map[string]bool) string {
	var parts []string
	for _, dim := range newDimensionSchema([]benchkit.Dimensions{dims}) {
		if !varying[dim.name] {
			continue
		}
		value := dim.value(dims)
		if dim.unset(value) {
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

// findDimension returns the dimension with the given name: a built-in
// dimension, or else the custom dimension of a valid custom name.
func findDimension(name string) (dimensionSpec, bool) {
	for _, dim := range builtinDimensions {
		if dim.name == name {
			return dim, true
		}
	}
	if dimensionNamePattern.MatchString(name) {
		return customDimensionSpec(name), true
	}
	return dimensionSpec{}, false
}

// dimensionLabel returns the axis label of the named dimension.
func dimensionLabel(name string) string {
	dim, _ := findDimension(name)
	return dim.label
}
