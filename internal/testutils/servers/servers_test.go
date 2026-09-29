package servers

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

// recordTB records Errorf calls without failing the test. stop reports a
// listener error through testing.TB, and the assertions below decide which
// errors are expected.
type recordTB struct {
	*testing.T
	errs []string
}

func (r *recordTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

// closeSeqListener returns errAt[n] from the nth Close and nil otherwise.
type closeSeqListener struct {
	closes int
	errAt  map[int]error
}

func (l *closeSeqListener) Close() error {
	l.closes++
	if err, ok := l.errAt[l.closes]; ok {
		return err
	}
	return nil
}

func (l *closeSeqListener) Accept() (net.Conn, error) { return nil, net.ErrClosed }

func (l *closeSeqListener) Addr() net.Addr { return closeSeqAddr{} }

type closeSeqAddr struct{}

func (closeSeqAddr) Network() string { return "tcp" }
func (closeSeqAddr) String() string  { return "127.0.0.1:9" }

// closeGate closes the listener inside Serve, then waits until Stop.
// That is the order grpc.Server uses when Serve returns before stop runs.
type closeGate struct {
	entered chan struct{}
	release chan struct{}
}

func (g *closeGate) Serve(lis net.Listener) error {
	err := lis.Close()
	close(g.entered)
	<-g.release
	return err
}

func (g *closeGate) Stop() { close(g.release) }

func TestServersStopIgnoresErrClosed(t *testing.T) {
	tests := []struct {
		name     string
		closeErr error
		wantErr  bool
	}{
		{name: "ErrClosed", closeErr: net.ErrClosed, wantErr: false},
		{name: "WrappedErrClosed", closeErr: fmt.Errorf("close listener: %w", net.ErrClosed), wantErr: false},
		{name: "Other", closeErr: errors.New("listen failed"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := &closeGate{entered: make(chan struct{}), release: make(chan struct{})}
			lis := &closeSeqListener{errAt: map[int]error{2: tt.closeErr}}
			state := &serverState{srv: srv, lis: lis, stopped: make(chan struct{})}
			go state.start(t)
			<-srv.entered
			tb := &recordTB{T: t}
			state.stop(tb)
			gotErr := len(tb.errs) > 0
			if gotErr != tt.wantErr {
				t.Errorf("stop errors = %v, wantErr %v", tb.errs, tt.wantErr)
			}
		})
	}
}
