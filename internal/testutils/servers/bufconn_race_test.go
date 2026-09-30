//go:build !integration

package servers

import (
	"context"
	"testing"

	"google.golang.org/grpc"
)

// TestStartDialRacesSecondStart verifies that a dial for t, as issued by the
// DialOptions dialer from a background gRPC connection attempt, is safe while
// a second Start for the same t populates its address map. Run it with -race.
func TestStartDialRacesSecondStart(t *testing.T) {
	_, stop1 := Start(t, 1, func(int) ServerIface { return grpc.NewServer() })
	t.Cleanup(func() { stop1() })

	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			default:
			}
			// Same steps as the dialer returned by DialOptions(t).
			d, err := globalBufconnRegistry.getDialer(t)
			if err != nil {
				continue
			}
			if c, err := d(context.Background(), "127.0.0.1:1"); err == nil {
				c.Close()
			}
		}
	}()
	_, stop2 := Start(t, 200, func(int) ServerIface { return grpc.NewServer() })
	close(done)
	<-finished
	t.Cleanup(func() { stop2() })
}
