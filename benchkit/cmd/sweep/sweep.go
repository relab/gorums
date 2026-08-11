package main

import (
	"fmt"
	"iter"

	"github.com/relab/gorums/benchkit"
)

// runSpec holds the dimensions and repetition for one benchmark run.
type runSpec struct {
	benchkit.Dimensions
	Rep int `json:"rep"` // repetition number, 1-based
}

// sweepConfig holds the parameter ranges for a sweep.
// The [params] method produces the Cartesian product of all combinations.
// An empty sendBuffers or recvBuffers contributes one zero value, which selects
// the benchmark binary's default.
type sweepConfig struct {
	numNodes    []int
	workers     []int
	payloads    []int
	rates       []int
	sendBuffers []int
	recvBuffers []int
	benchmarks  []string
	streamModes []string
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

// params returns an iterator over all swept benchmark parameter combinations.
func (sc sweepConfig) params() iter.Seq[runSpec] {
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
									for _, benchmark := range sc.benchmarks {
										for _, streamMode := range streamModes {
											if !yield(runSpec{
												Dimensions: benchkit.Dimensions{
													Benchmark:  benchmark,
													Nodes:      n,
													Workers:    workers,
													Payload:    payload,
													Rate:       rate,
													SendBuffer: sendBuffer,
													RecvBuffer: recvBuffer,
													StreamMode: streamMode,
												},
												Rep: rep,
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

// runBase returns the base filename prefix for a run's output files,
// matching the naming convention the report generator expects.
// Format: <label>_<Benchmark>_N<n>_W<workers>_P<payload>[_R<rate>][_SB<send>][_RB<recv>]_S<stream>_r<rep>
func runBase(label string, spec runSpec) string {
	base := fmt.Sprintf("%s_%s_N%d_W%d_P%d", label, spec.Benchmark, spec.Nodes, spec.Workers, spec.Payload)
	if spec.Rate > 0 {
		base += fmt.Sprintf("_R%d", spec.Rate)
	}
	if spec.SendBuffer != 0 {
		base += fmt.Sprintf("_SB%d", spec.SendBuffer)
	}
	if spec.RecvBuffer != 0 {
		base += fmt.Sprintf("_RB%d", spec.RecvBuffer)
	}
	if spec.StreamMode != "" {
		base += fmt.Sprintf("_S%s", spec.StreamMode)
	}
	base += fmt.Sprintf("_r%d", spec.Rep)
	return base
}

func nonDefaultStreamModes(modes []string) bool {
	if len(modes) == 0 {
		return false
	}
	return len(modes) != 1 || modes[0] != "dual"
}
