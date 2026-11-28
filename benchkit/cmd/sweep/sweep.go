package main

import (
	"fmt"
	"iter"
	"slices"
	"strings"

	"github.com/relab/gorums/benchkit"
)

// runSpec holds the dimensions and repetition for one benchmark run.
type runSpec struct {
	benchkit.Dimensions
	Rep int `json:"rep"` // repetition number, 1-based
}

// sweepConfig holds the parameter ranges for a sweep.
// The [sweepConfig.runSpecs] method produces the Cartesian product of all combinations.
// An empty sendBuffers or recvBuffers contributes one zero value, which selects
// the benchmark binary's default. dims holds the custom dimensions, sorted by
// name; each joins the product with its values.
type sweepConfig struct {
	numNodes    []int
	workers     []int
	payloads    []int
	rates       []int
	sendBuffers []int
	recvBuffers []int
	benchmarks  []string
	streamModes []string
	dims        []dimDecl
	reps        int
}

// bufferValues returns sizes, or a single default-selecting zero when sizes is
// empty, so an unswept buffer axis contributes exactly one combination.
func bufferValues(sizes []int) []int {
	if len(sizes) == 0 {
		return []int{0}
	}
	return sizes
}

// customCombinations returns an iterator over the Cartesian product of the custom
// dimension values, one [benchkit.CustomDimensions] per combination, with the last
// dimension varying fastest. A sweep without custom dimensions has one empty
// combination. The iterator holds one value index per dimension.
func (sc sweepConfig) customCombinations() iter.Seq[benchkit.CustomDimensions] {
	return func(yield func(benchkit.CustomDimensions) bool) {
		if slices.ContainsFunc(sc.dims, func(d dimDecl) bool { return len(d.values) == 0 }) {
			return
		}
		next := make([]int, len(sc.dims))
		combo := make(map[string]string, len(sc.dims))
		for {
			for i, dim := range sc.dims {
				combo[dim.name] = dim.values[next[i]]
			}
			if !yield(benchkit.NewCustomDimensions(combo)) {
				return
			}
			// Advance the indexes like an odometer, the last dimension first.
			i := len(next) - 1
			for ; i >= 0; i-- {
				if next[i]++; next[i] < len(sc.dims[i].values) {
					break
				}
				next[i] = 0
			}
			if i < 0 {
				return
			}
		}
	}
}

// runSpecs returns an iterator over all swept benchmark parameter combinations.
func (sc sweepConfig) runSpecs() iter.Seq[runSpec] {
	return func(yield func(runSpec) bool) {
		reps := max(sc.reps, 1)
		streamModes := sc.streamModes
		if len(streamModes) == 0 {
			streamModes = []string{"dual"}
		}
		for rep := 1; rep <= reps; rep++ {
			for _, n := range sc.numNodes {
				for _, workers := range sc.workers {
					for _, payload := range sc.payloads {
						for _, rate := range sc.rates {
							for _, sendBuffer := range bufferValues(sc.sendBuffers) {
								for _, recvBuffer := range bufferValues(sc.recvBuffers) {
									for customDims := range sc.customCombinations() {
										for _, benchmark := range sc.benchmarks {
											for _, streamMode := range streamModes {
												if !yield(runSpec{
													Benchmark:  benchmark,
													Nodes:      n,
													Workers:    workers,
													Payload:    payload,
													Rate:       rate,
													SendBuffer: sendBuffer,
													RecvBuffer: recvBuffer,
													StreamMode: streamMode,
													Custom:     customDims,
													Rep:        rep,
												}) {
													return
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

// runBase returns the base filename prefix for a run's output files,
// matching the naming convention the report generator expects.
// Format: <label>_<Benchmark>_N<n>_W<workers>_P<payload>[_R<rate>][_SB<send>][_RB<recv>][_<name>=<value>...]_S<stream>_r<rep>
// The custom dimensions appear sorted by name.
func runBase(label string, spec runSpec) string {
	var base strings.Builder
	fmt.Fprintf(&base, "%s_%s_N%d_W%d_P%d", label, spec.Benchmark, spec.Nodes, spec.Workers, spec.Payload)
	if spec.Rate > 0 {
		fmt.Fprintf(&base, "_R%d", spec.Rate)
	}
	if spec.SendBuffer != 0 {
		fmt.Fprintf(&base, "_SB%d", spec.SendBuffer)
	}
	if spec.RecvBuffer != 0 {
		fmt.Fprintf(&base, "_RB%d", spec.RecvBuffer)
	}
	for name, value := range spec.Custom.All() {
		fmt.Fprintf(&base, "_%s=%s", name, value)
	}
	if spec.StreamMode != "" {
		fmt.Fprintf(&base, "_S%s", spec.StreamMode)
	}
	fmt.Fprintf(&base, "_r%d", spec.Rep)
	return base.String()
}

func nonDefaultStreamModes(modes []string) bool {
	if len(modes) == 0 {
		return false
	}
	return len(modes) != 1 || modes[0] != "dual"
}
