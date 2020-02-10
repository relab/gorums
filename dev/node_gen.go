// Code generated by protoc-gen-gorums. DO NOT EDIT.
// Source file to edit is: dev/storage.proto
// Template file to edit is: node.tmpl

package dev

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc"
)

// Node encapsulates the state of a node on which a remote procedure call
// can be made.
type Node struct {
	// Only assigned at creation.
	id     uint32
	addr   string
	conn   *grpc.ClientConn
	logger *log.Logger

	StorageClient StorageClient

	WriteAsyncClient   Storage_WriteAsyncClient
	WriteOrderedClient Storage_WriteOrderedClient
	writeOrderedSend   chan *State
	writeOrderedRecv   map[uint32]chan *internalWriteResponse
	writeOrderedLock   *sync.RWMutex

	mu      sync.Mutex
	lastErr error
	latency time.Duration
}

func (n *Node) connect(opts managerOptions) error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, opts.grpcDialOpts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %v", err)
	}

	n.StorageClient = NewStorageClient(n.conn)

	n.WriteAsyncClient, err = n.StorageClient.WriteAsync(context.Background())
	if err != nil {
		return fmt.Errorf("stream creation failed: %v", err)
	}

	n.WriteOrderedClient, err = n.StorageClient.WriteOrdered(context.Background())
	if err != nil {
		return fmt.Errorf("stream creation failed: %v", err)
	}

	go n.writeOrderedSendMsgs()
	go n.writeOrderedRecvMsgs()

	return nil
}

func (n *Node) close() error {
	_, _ = n.WriteAsyncClient.CloseAndRecv()
	_ = n.WriteOrderedClient.CloseSend()
	close(n.writeOrderedSend)

	if err := n.conn.Close(); err != nil {
		if n.logger != nil {
			n.logger.Printf("%d: conn close error: %v", n.id, err)
		}
		return fmt.Errorf("%d: conn close error: %v", n.id, err)
	}
	return nil
}

func (n *Node) writeOrderedSendMsgs() {
	for msg := range n.writeOrderedSend {
		err := n.WriteOrderedClient.SendMsg(msg)
		if err != nil {
			if err != io.EOF {
				if n.logger != nil {
					n.logger.Printf("%d: WriteOrdered send error: %v", n.id, err)
				}
				n.setLastErr(err)
			}
			return
		}
	}
}

func (n *Node) writeOrderedRecvMsgs() {
	for {
		msg := new(WriteResponse)
		err := n.WriteOrderedClient.RecvMsg(msg)
		if err != nil {
			if err != io.EOF {
				if n.logger != nil {
					n.logger.Printf("%d: WriteOrdered receive error: %v", n.id, err)
				}
				n.setLastErr(err)
			}
			return
		}
		id := msg.GorumsMessageID
		n.writeOrderedLock.RLock()
		if c, ok := n.writeOrderedRecv[id]; ok {
			c <- &internalWriteResponse{n.id, msg, nil}
		}
		n.writeOrderedLock.RUnlock()
	}
}
