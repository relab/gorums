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

func (b *Broadcast) Multiparty(req *Request) {
	b.SetBroadcastValues("dev.ZorumsService.Multiparty", req)
}