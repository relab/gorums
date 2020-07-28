package gorums

import (
	"fmt"
	"time"

	"golang.org/x/net/trace"
)

type traceInfo struct {
	trace.Trace
	firstLine firstLine
}

type firstLine struct {
	deadline time.Duration
	cid      uint32
}

func (f firstLine) String() string {
	if f.deadline != 0 {
		return fmt.Sprintf("QC: to config%d deadline: %d", f.cid, f.deadline)
	}
	return fmt.Sprintf("QC: to config%d deadline: none", f.cid)
}

type payload struct {
	sent bool
	id   uint32
	msg  interface{}
}

func (p payload) String() string {
	if p.sent {
		return fmt.Sprintf("sent: %v", p.msg)
	}
	return fmt.Sprintf("recv from %d: %v", p.id, p.msg)
}

type qcresult struct {
	ids   []uint32
	reply interface{}
	err   error
}

func (q qcresult) String() string {
	if q.err == nil {
		return fmt.Sprintf("recv QC reply: ids: %v, reply: %v", q.ids, q.reply)
	}
	return fmt.Sprintf("recv QC reply: ids: %v, reply: %v, error: %v", q.ids, q.reply, q.err)
}
