// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: zorums.proto

package dev

import (
	gorums "github.com/relab/gorums"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

func (b *Broadcast) QuorumCallWithBroadcast(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	b.sp.BroadcastHandler("dev.ZorumsService.QuorumCallWithBroadcast", req, b.metadata, options)
}

func (b *Broadcast) BroadcastInternal(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	b.sp.BroadcastHandler("dev.ZorumsService.BroadcastInternal", req, b.metadata, options)
}

func (b *Broadcast) BroadcastWithClientHandlerAndBroadcastOption(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	b.sp.BroadcastHandler("dev.ZorumsService.BroadcastWithClientHandlerAndBroadcastOption", req, b.metadata, options)
}
