// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	empty "github.com/golang/protobuf/ptypes/empty"
	ordering "github.com/relab/gorums/ordering"
)

func (n *Node) Unicast(in *Request) error {
	msgID := n.nextMsgID()
	metadata := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  unicastMethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
	n.sendQ <- msg
	return nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

func (n *Node) Unicast2(in *Request) error {
	msgID := n.nextMsgID()
	metadata := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  unicast2MethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
	n.sendQ <- msg
	return nil
}

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

func (n *Node) UnicastConcurrent(in *Request) error {
	msgID := n.nextMsgID()
	metadata := &ordering.Metadata{
		MessageID: msgID,
		MethodID:  unicastConcurrentMethodID,
	}
	msg := &gorumsMessage{metadata: metadata, message: in}
	n.sendQ <- msg
	return nil
}