// Package benchkit is a toolkit for load-testing and measuring distributed
// systems.
//
// It provides the building blocks for a benchmark run: load generation
// (pacer, ticker), latency and throughput measurement (HDR histograms, time
// series, summary statistics), fault injection, clock synchronization, a
// control server for coordinating remote workers, and aggregation and
// reporting of the collected results.
package benchkit
