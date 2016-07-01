package dev

import (
	"log"
	"sync"

	"golang.org/x/net/context"
)

// Manager manages a pool of node configurations on which quorum remote
// procedure calls can be made.
type Manager struct {
	sync.RWMutex

	nodes   map[uint32]*Node
	configs map[uint32]*Configuration

	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions

	readqf  ReadQuorumFn
	writeqf WriteQuorumFn

	// Why are the stream clients put here?
	// They should be in the node struct, but the node struct is now
	// totally generic and does not need code generation (which is
	// convenient). The manager type already needs code generation due to
	// storing the quorum picker functions. We therefore store the client
	// streams here to keep the code generation more simple.
	//
	// TODO: Put into Node type. Add _gen to node.go
	writeAsyncClients map[uint32]Register_WriteAsyncClient
}

func (m *Manager) setDefaultQuorumFuncs() {
	if m.opts.readqf != nil {
		m.readqf = m.opts.readqf
	} else {
		m.readqf = func(c *Configuration, replies []*State) (*State, bool) {
			if len(replies) < c.Quorum() {
				return nil, false
			}
			return replies[0], true
		}
	}
	if m.opts.writeqf != nil {
		m.writeqf = m.opts.writeqf
	} else {
		m.writeqf = func(c *Configuration, replies []*WriteResponse) (*WriteResponse, bool) {
			if len(replies) < c.Quorum() {
				return nil, false
			}
			return replies[0], true
		}
	}
}

func (m *Manager) createStreamClients() error {
	if m.opts.noConnect {
		return nil
	}

	m.writeAsyncClients = make(map[uint32]Register_WriteAsyncClient)
	for _, node := range m.nodes {
		client := NewRegisterClient(node.conn)
		writeAsyncClient, err := client.WriteAsync(context.Background())
		if err != nil {
			return err
		}
		m.writeAsyncClients[node.id] = writeAsyncClient
	}

	return nil
}

func (m *Manager) closeStreamClients() {
	if m.opts.noConnect {
		return
	}

	for id, client := range m.writeAsyncClients {
		_, err := client.CloseAndRecv()
		if err == nil {
			continue
		}
		if m.logger != nil {
			m.logger.Printf("node %d: error closing writeAsync client: %v", id, err)
		}
	}
}
