package dev_test

import (
	context "context"
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/cmd/protoc-gen-gorums/dev"
)

const quorumCallMethod = "dev.ZorumsService.QuorumCall"

func TestQuorumCallWithDefaultQuorumFunc(t *testing.T) {
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

	// Create configuration using helper - now we just use RawConfiguration
	rawCfg := gorums.NewConfig(t, addrs)

	ctx := testContext(t, 2*time.Second)

	req := &dev.Request{}
	req.SetValue("test")

	// Call QuorumCall with default MajorityQuorum
	resp, err := dev.QuorumCall(ctx, rawCfg, req)
	if err != nil {
		t.Fatalf("QuorumCall failed: %v", err)
	}

	// The server returns len(req.Value) which is 4 for "test"
	if resp.GetResult() != 4 {
		t.Errorf("Expected result 4, got %d", resp.GetResult())
	}
}

func TestQuorumCallWithCustomQuorumFunc(t *testing.T) {
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
	rawCfg := gorums.NewConfig(t, addrs)

	// Custom QuorumFunc that requires all responses
	customQF := func(ctx *gorums.ClientCtx[*dev.Request, *dev.Response]) (*dev.Response, error) {
		var lastResp *dev.Response
		count := 0
		for result := range ctx.Responses() {
			if result.Err == nil {
				lastResp = result.Value
				count++
			}
		}
		if count < ctx.Size() {
			return nil, gorums.QuorumCallError{} // Not all responded
		}
		return lastResp, nil
	}

	ctx := testContext(t, 2*time.Second)

	req := &dev.Request{}
	req.SetValue("test")

	// Call QuorumCall with custom quorum function via call option
	resp, err := dev.QuorumCall(ctx, rawCfg, req, gorums.WithQuorumFunc(customQF))
	if err != nil {
		t.Fatalf("QuorumCall with custom QF failed: %v", err)
	}

	// The server returns len(req.Value) which is 4 for "test"
	if resp.GetResult() != 4 {
		t.Errorf("Expected result 4, got %d", resp.GetResult())
	}
}

// TestQuorumCallWithQuorumSpecFunc tests using a legacy QuorumSpec-style function
func TestQuorumCallWithQuorumSpecFunc(t *testing.T) {
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

	rawCfg := gorums.NewConfig(t, addrs)

	// Legacy-style quorum function (like the old QuorumSpec methods)
	legacyQF := func(_ *dev.Request, replies map[uint32]*dev.Response) (*dev.Response, bool) {
		if len(replies) < 2 {
			return nil, false
		}
		// Return any reply
		for _, r := range replies {
			return r, true
		}
		return nil, false
	}

	ctx := testContext(t, 2*time.Second)

	req := &dev.Request{}
	req.SetValue("hello")

	// Use QuorumSpecFunc to adapt the legacy function
	resp, err := dev.QuorumCall(ctx, rawCfg, req,
		gorums.WithQuorumFunc(gorums.QuorumSpecFunc(legacyQF)))
	if err != nil {
		t.Fatalf("QuorumCall with QuorumSpecFunc failed: %v", err)
	}

	// The server returns len(req.Value) which is 5 for "hello"
	if resp.GetResult() != 5 {
		t.Errorf("Expected result 5, got %d", resp.GetResult())
	}
}

func testContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}
