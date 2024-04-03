package gorums

import (
	"time"

	"github.com/relab/gorums/ordering"
)

type content interface {
	Send()
	Remove()
}

type clientReq struct {
	ctx        ServerCtx
	finished   chan<- *Message
	metadata   *ordering.Metadata
	senderType string
	//methods   []string
	//counts    map[string]uint64
	timestamp time.Time
	doneChan  chan struct{}
}

type BroadcastRouter struct {
	routes   map[string]content
	doneChan chan struct{}
}
