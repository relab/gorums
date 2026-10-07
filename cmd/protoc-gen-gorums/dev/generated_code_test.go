// Package dev_test contains integration tests for the generated Gorums code.
// These tests validate that the protoc-gen-gorums code generator produces
// correct and functional code. They exercise the generated server
// registration, the generated QuorumCall method, and its terminal methods
// (Majority, All, Threshold) along with custom aggregation patterns using
// CollectAll.
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
	"github.com/relab/gorums/gorumstest"
)

// quorumCallServer implements the QuorumCall method of the generated
// [dev.ZorumsServiceServer] interface. It returns the length of the request
// value. The embedded interface is nil, so the other methods panic if called.
type quorumCallServer struct {
	dev.ZorumsServiceServer
}

func (quorumCallServer) QuorumCall(_ gorums.ServerContext, req *dev.Request) (*dev.Response, error) {
	resp := &dev.Response{}
	resp.SetResult(int64(len(req.GetValue())))
	return resp, nil
}

func newQuorumCallServer(_ int) gorums.ServerIface {
	srv := gorums.NewServer()
	dev.RegisterZorumsServiceServer(srv, quorumCallServer{})
	return srv
}

func TestGeneratedCodeQuorumCall(t *testing.T) {
	tests := []struct {
		name  string
		value string
		call  func(*dev.ConfigContext, *dev.Request) (int64, error)
		want  int64
	}{
		{
			name:  "Majority",
			value: "test",
			call: func(ctx *dev.ConfigContext, req *dev.Request) (int64, error) {
				resp, err := dev.QuorumCall(ctx, req).Majority()
				return resp.GetResult(), err
			},
			want: 4,
		},
		{
			name:  "All",
			value: "test",
			call: func(ctx *dev.ConfigContext, req *dev.Request) (int64, error) {
				resp, err := dev.QuorumCall(ctx, req).All()
				return resp.GetResult(), err
			},
			want: 4,
		},
		{
			name:  "Threshold",
			value: "hello",
			call: func(ctx *dev.ConfigContext, req *dev.Request) (int64, error) {
				resp, err := dev.QuorumCall(ctx, req).Threshold(2)
				return resp.GetResult(), err
			},
			want: 5,
		},
		{
			// Each of the 3 servers returns len("hello"), so the sum is 15.
			name:  "CollectAllSum",
			value: "hello",
			call: func(ctx *dev.ConfigContext, req *dev.Request) (int64, error) {
				var total int64
				for _, resp := range dev.QuorumCall(ctx, req).Results().CollectAll() {
					total += resp.GetResult()
				}
				return total, nil
			},
			want: 15,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gorumstest.Config(t, 3, newQuorumCallServer)
			ctx := config.Context(gorumstest.Context(t, 2*time.Second))

			req := &dev.Request{}
			req.SetValue(tt.value)
			got, err := tt.call(ctx, req)
			if err != nil {
				t.Fatalf("QuorumCall failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("QuorumCall result = %d, want %d", got, tt.want)
			}
		})
	}
}
