package dev_test

import (
	"context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

const quorumCallMethod = "dev.ZorumsService.QuorumCall"

func TestQuorumCallWithMajority(t *testing.T) {
	// Setup servers with handler for the QuorumCall method
	addrs, teardown := gorums.TestSetup(t, 3, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(quorumCallMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*dev.Request](in)
			resp := &dev.Response{}
			resp.SetResult(int64(len(req.GetValue())))
			return gorums.NewResponseMessage(in.GetMetadata(), resp), nil
		})
		return srv
	})
	t.Cleanup(teardown)

	// Create configuration using helper
	cfg := gorums.NewTestConfig(t, addrs)

	ctx := gorums.WithConfigContext(testContext(t, 2*time.Second), cfg)

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
	// Setup servers
	addrs, teardown := gorums.TestSetup(t, 3, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(quorumCallMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*dev.Request](in)
			resp := &dev.Response{}
			resp.SetResult(int64(len(req.GetValue())))
			return gorums.NewResponseMessage(in.GetMetadata(), resp), nil
		})
		return srv
	})
	t.Cleanup(teardown)

	// Create configuration using helper
	cfg := gorums.NewTestConfig(t, addrs)

	ctx := gorums.WithConfigContext(testContext(t, 2*time.Second), cfg)

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
	// Setup servers
	addrs, teardown := gorums.TestSetup(t, 3, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(quorumCallMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*dev.Request](in)
			resp := &dev.Response{}
			resp.SetResult(int64(len(req.GetValue())))
			return gorums.NewResponseMessage(in.GetMetadata(), resp), nil
		})
		return srv
	})
	t.Cleanup(teardown)

	cfg := gorums.NewTestConfig(t, addrs)

	ctx := gorums.WithConfigContext(testContext(t, 2*time.Second), cfg)

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
	// Setup servers
	addrs, teardown := gorums.TestSetup(t, 3, func(_ int) gorums.ServerIface {
		srv := gorums.NewServer()
		srv.RegisterHandler(quorumCallMethod, func(_ gorums.ServerCtx, in *gorums.Message) (*gorums.Message, error) {
			req := gorums.AsProto[*dev.Request](in)
			resp := &dev.Response{}
			resp.SetResult(int64(len(req.GetValue())))
			return gorums.NewResponseMessage(in.GetMetadata(), resp), nil
		})
		return srv
	})
	t.Cleanup(teardown)

	cfg := gorums.NewTestConfig(t, addrs)

	ctx := gorums.WithConfigContext(testContext(t, 2*time.Second), cfg)

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

func testContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}
