package main

import (
	"context"
	"io"
	"strconv"
	"testing"
	"time"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"github.com/relab/gorums/gorumstest"
)

func TestDisseminatePeriodicUpdates(t *testing.T) {
	for _, tt := range []struct {
		name  string
		count uint64
	}{
		{name: "Count", count: 2},
		{name: "Cancel"},
	} {
		t.Run(tt.name, func(t *testing.T) { testDisseminate(t, tt.count) })
	}
}

func testDisseminate(t *testing.T, count uint64) {
	t.Helper()
	stores := make([]*storageServer, 2)
	cfg := gorumstest.Config(t, len(stores), func(i int) gorumstest.ServerIface {
		stores[i] = newStorageServer(io.Discard, "")
		srv := gorums.NewServer()
		pb.RegisterStorageServer(srv, stores[i])
		return srv
	})
	ctx, cancel := context.WithCancel(gorumstest.Context(t, 5*time.Second))
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- disseminate(ctx, cfg, count, time.Millisecond, io.Discard) }()
	if !gorumstest.WaitUntil(t, time.Second, func() bool {
		for _, store := range stores {
			store.mut.Lock()
			value, _ := strconv.Atoi(store.storage["counter"].Value)
			store.mut.Unlock()
			if value < 2 {
				return false
			}
		}
		return true
	}) {
		t.Error("periodic updates did not reach every node")
	}
	if count == 0 {
		cancel()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("dissemination did not stop")
	}
	if count > 0 {
		for _, store := range stores {
			store.mut.Lock()
			value := store.storage["counter"].Value
			store.mut.Unlock()
			if value != strconv.FormatUint(count, 10) {
				t.Errorf("counter = %s, want %d", value, count)
			}
		}
	}
}
