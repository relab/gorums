// Package dev_test contains integration tests for the generated Gorums code.
// These tests validate that the protoc-gen-gorums code generator produces
// correct and functional code. They exercise the generated QuorumCall method
// and its terminal methods (Majority, All, Threshold) along with custom
// aggregation patterns using CollectAll.
//
// NOTE: These tests are intentionally separate from the core library tests
// in the repository root. While they test similar functionality, they serve
// a different purpose: verifying the code generation pipeline end-to-end.
package dev_test

import (
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

const quorumCallMethod = "dev.ZorumsService.QuorumCall"

func quorumCallServer(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	srv.RegisterHandler(quorumCallMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
		req := gorums.AsProto[*dev.Request](in)
		resp := &dev.Response{}
		resp.SetResult(int64(len(req.GetValue())))
		return gorums.NewResponseMessage(in, resp), nil
	})
	return srv
}

func TestQuorumCallWithMajority(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, quorumCallServer)
	ctx := config.Context(gorums.TestContext(t, 2*time.Second))

	req := &dev.Request{}
	req.SetValue("test")

	// Call QuorumCall and wait for a majority to respond
	resp, err := dev.QuorumCall(ctx, req).Majority()
	if err != nil {
		t.Fatalf("QuorumCall.Majority() failed: %v", err)
	}

	// The server returns len(req.Value) which is 4 for "test"
	if resp.GetResult() != 4 {
		t.Errorf("Expected result 4, got %d", resp.GetResult())
	}
}

func TestQuorumCallWithAll(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, quorumCallServer)
	ctx := config.Context(gorums.TestContext(t, 2*time.Second))

	req := &dev.Request{}
	req.SetValue("test")

	// Call QuorumCall and wait for all responses
	resp, err := dev.QuorumCall(ctx, req).All()
	if err != nil {
		t.Fatalf("QuorumCall.All() failed: %v", err)
	}

	// The server returns len(req.Value) which is 4 for "test"
	if resp.GetResult() != 4 {
		t.Errorf("Expected result 4, got %d", resp.GetResult())
	}
}

func TestQuorumCallWithThreshold(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, quorumCallServer)
	ctx := config.Context(gorums.TestContext(t, 2*time.Second))

	req := &dev.Request{}
	req.SetValue("hello")

	// Use Threshold to wait for at least 2 responses
	resp, err := dev.QuorumCall(ctx, req).Threshold(2)
	if err != nil {
		t.Fatalf("QuorumCall.Threshold() failed: %v", err)
	}

	// The server returns len(req.Value) which is 5 for "hello"
	if resp.GetResult() != 5 {
		t.Errorf("Expected result 5, got %d", resp.GetResult())
	}
}

func TestQuorumCallWithCustomAggregation(t *testing.T) {
	config := gorums.TestConfiguration(t, 3, quorumCallServer)
	ctx := config.Context(gorums.TestContext(t, 2*time.Second))

	req := &dev.Request{}
	req.SetValue("hello")

	// Use CollectAll for custom aggregation (sum all results)
	responses := dev.QuorumCall(ctx, req)
	results := responses.CollectAll()

	var total int64
	for _, resp := range results {
		total += resp.GetResult()
	}

	// Each server returns 5 (len("hello")), so 3 servers = 15
	if total != 15 {
		t.Errorf("Expected total result 15, got %d", total)
	}
}
