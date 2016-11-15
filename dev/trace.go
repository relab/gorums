package dev

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"golang.org/x/net/trace"
)

type traceInfo struct {
	tr        trace.Trace
	firstLine firstLine
}

type firstLine struct {
	deadline time.Duration
	cid      uint32
}

func (f *firstLine) String() string {
	var line bytes.Buffer
	io.WriteString(&line, "QC: to config")
	fmt.Fprintf(&line, "%v deadline:", f.cid)
	if f.deadline != 0 {
		fmt.Fprint(&line, f.deadline)
	} else {
		io.WriteString(&line, "none")
	}
	return line.String()
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
	var out bytes.Buffer
	io.WriteString(&out, "recv QC reply: ")
	fmt.Fprintf(&out, "ids: %v, ", q.ids)
	fmt.Fprintf(&out, "reply: %v ", q.reply)
	if q.err != nil {
		fmt.Fprintf(&out, ", error: %v", q.err)
	}
	return out.String()
}
