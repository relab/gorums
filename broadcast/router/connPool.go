package router

import (
	"github.com/relab/gorums/broadcast/dtos"
	"sync"
)

// ConnPool is used to persist connection from the server to other clients.
// This significantly increases performance because it reuses connections for separate
// messages.
type ConnPool struct {
	mut     sync.Mutex
	clients map[string]*dtos.Client
}

func newConnPool() *ConnPool {
	return &ConnPool{
		clients: make(map[string]*dtos.Client),
	}
}

func (cp *ConnPool) getClient(addr string) (*dtos.Client, bool) {
	cp.mut.Lock()
	defer cp.mut.Unlock()
	client, ok := cp.clients[addr]
	return client, ok
}

func (cp *ConnPool) addClient(addr string, client *dtos.Client) {
	cp.mut.Lock()
	defer cp.mut.Unlock()
	cp.clients[addr] = client
}

func (cp *ConnPool) Close() error {
	var err error = nil
	for _, client := range cp.clients {
		clientErr := client.Close()
		if clientErr != nil {
			err = clientErr
		}
	}
	return err
}
