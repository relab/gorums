package multiservice_test

import (
	"testing"
	"time"

	"github.com/relab/gorums"
	"github.com/relab/gorums/gorumstest"
	"github.com/relab/gorums/internal/tests/multiservice"
)

type alphaSrv struct{}

func (alphaSrv) Heartbeat(_ gorums.ServerContext, in *multiservice.Request) (*multiservice.Response, error) {
	return multiservice.Response_builder{Value: "alpha:" + in.GetValue()}.Build(), nil
}

func (alphaSrv) Read(_ gorums.ServerContext, in *multiservice.Request) (*multiservice.Response, error) {
	return multiservice.Response_builder{Value: "alpha-read:" + in.GetValue()}.Build(), nil
}

type betaSrv struct{}

func (betaSrv) Heartbeat(_ gorums.ServerContext, in *multiservice.Request) (*multiservice.Response, error) {
	return multiservice.Response_builder{Value: "beta:" + in.GetValue()}.Build(), nil
}

func (betaSrv) Write(_ gorums.ServerContext, in *multiservice.Request) (*multiservice.Response, error) {
	return multiservice.Response_builder{Value: "beta-write:" + in.GetValue()}.Build(), nil
}

// TestMultiServiceSharedConfig checks that two services of one proto file
// share one server and one configuration, and that each call reaches the
// service that declares it, also when both services have a method of the
// same name.
func TestMultiServiceSharedConfig(t *testing.T) {
	config := gorumstest.Config(t, 3, func(_ int) gorumstest.ServerIface {
		srv := gorums.NewServer()
		multiservice.RegisterAlphaServer(srv, alphaSrv{})
		multiservice.RegisterBetaServer(srv, betaSrv{})
		return srv
	})
	alpha := multiservice.NewAlphaClient(config)
	beta := multiservice.NewBetaClient(config)
	req := multiservice.Request_builder{Value: "ping"}.Build()

	tests := []struct {
		name string
		call func(t *testing.T) (*multiservice.Response, error)
		want string
	}{
		{
			name: "AlphaHeartbeat",
			call: func(t *testing.T) (*multiservice.Response, error) {
				return alpha.Heartbeat(gorumstest.Context(t, time.Second), req).Majority()
			},
			want: "alpha:ping",
		},
		{
			name: "BetaHeartbeat",
			call: func(t *testing.T) (*multiservice.Response, error) {
				return beta.Heartbeat(gorumstest.Context(t, time.Second), req).Majority()
			},
			want: "beta:ping",
		},
		{
			name: "AlphaRead",
			call: func(t *testing.T) (*multiservice.Response, error) {
				return multiservice.NewAlphaNodeClient(config.Nodes()[0]).Read(gorumstest.Context(t, time.Second), req)
			},
			want: "alpha-read:ping",
		},
		{
			name: "BetaWrite",
			call: func(t *testing.T) (*multiservice.Response, error) {
				return beta.Write(gorumstest.Context(t, time.Second), req).Majority()
			},
			want: "beta-write:ping",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.call(t)
			if err != nil {
				t.Fatalf("call failed: %v", err)
			}
			if got := resp.GetValue(); got != tt.want {
				t.Errorf("response = %q, want %q", got, tt.want)
			}
		})
	}
}
