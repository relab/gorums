package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/relab/gorums"
	pb "github.com/relab/gorums/examples/storage/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r repl) tick(args []string) {
	var count uint64
	if len(args) > 1 {
		fmt.Println("Usage: tick [positive count]")
		return
	}
	if len(args) == 1 {
		var err error
		count, err = strconv.ParseUint(args[0], 10, 64)
		if err != nil || count == 0 {
			fmt.Println("Tick count must be a positive integer.")
			return
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	fmt.Println("Sending a counter update every second. Press Ctrl-C to stop.")
	if err := disseminate(ctx, r.cfg, count, time.Second, os.Stdout); err != nil {
		fmt.Printf("Tick error: %v\n", err)
	}
}

// disseminate publishes the current counter on each tick. Each update carries
// the full value, so a later update repairs a missed write. Storage servers use
// the timestamp to keep the newest value. It stops after count ticks or when
// ctx ends; a count of zero runs until ctx ends.
func disseminate(ctx context.Context, cfg gorums.Config, count uint64, interval time.Duration, out io.Writer) error {
	client := pb.NewStorageClient(cfg)
	logger := log.New(out, "dissemination: ", 0)
	rejected := make(map[gorums.ID]uint64)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var counter uint64
	for count == 0 || counter < count {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			counter++
			req := pb.WriteRequest_builder{
				Key: "counter", Value: fmt.Sprint(counter), Time: timestamppb.New(now),
			}.Build()
			// Send reports admission errors at once; the event loop can then
			// process its next tick while Gorums writes accepted requests.
			err := client.WriteMulticast(ctx, req).Send()
			admitted := cfg.Size()
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				qcErr, ok := errors.AsType[gorums.QuorumCallError](err)
				if !ok {
					return err
				}
				admitted -= qcErr.NumErrors()
				for id, nodeErr := range qcErr.NodeErrors() {
					rejected[id]++
					logger.Printf("node %d: admission error: %v (rejected updates: %d)", id, nodeErr, rejected[id])
				}
			}
			logger.Printf("counter=%d admitted=%d/%d", counter, admitted, cfg.Size())
		}
	}
	return nil
}
