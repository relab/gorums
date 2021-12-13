// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.6.0-devel
// 	protoc            v3.17.3
// source: zorums.proto

package dev

import (
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(6 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 6)
)

type internalEmpty struct {
	nid   uint32
	reply *emptypb.Empty
	err   error
}

type internalResponse struct {
	nid   uint32
	reply *Response
	err   error
}

// AsyncEmpty is a async object for processing replies.
type AsyncEmpty struct {
	*gorums.Async
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *AsyncEmpty) Get() (*emptypb.Empty, error) {
	resp, err := f.Async.Get()
	if err != nil {
		return nil, err
	}
	return resp.(*emptypb.Empty), err
}

// AsyncMyResponse is a async object for processing replies.
type AsyncMyResponse struct {
	*gorums.Async
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *AsyncMyResponse) Get() (*MyResponse, error) {
	resp, err := f.Async.Get()
	if err != nil {
		return nil, err
	}
	return resp.(*MyResponse), err
}

// AsyncResponse is a async object for processing replies.
type AsyncResponse struct {
	*gorums.Async
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *AsyncResponse) Get() (*Response, error) {
	resp, err := f.Async.Get()
	if err != nil {
		return nil, err
	}
	return resp.(*Response), err
}

// CorrectableEmpty is a correctable object for processing replies.
type CorrectableEmpty struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableEmpty) Get() (*emptypb.Empty, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*emptypb.Empty), level, err
}

// CorrectableMyResponse is a correctable object for processing replies.
type CorrectableMyResponse struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableMyResponse) Get() (*MyResponse, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*MyResponse), level, err
}

// CorrectableResponse is a correctable object for processing replies.
type CorrectableResponse struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableResponse) Get() (*Response, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*Response), level, err
}

// CorrectableStreamEmpty is a correctable object for processing replies.
type CorrectableStreamEmpty struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamEmpty) Get() (*emptypb.Empty, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*emptypb.Empty), level, err
}

// CorrectableStreamMyResponse is a correctable object for processing replies.
type CorrectableStreamMyResponse struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamMyResponse) Get() (*MyResponse, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*MyResponse), level, err
}

// CorrectableStreamResponse is a correctable object for processing replies.
type CorrectableStreamResponse struct {
	*gorums.Correctable
}

// Get returns the reply, level and any error associated with the
// called method. The method does not block until a (possibly
// intermediate) reply or error is available. Level is set to LevelNotSet if no
// reply has yet been received. The Done or Watch methods should be used to
// ensure that a reply is available.
func (c *CorrectableStreamResponse) Get() (*Response, int, error) {
	resp, level, err := c.Correctable.Get()
	if err != nil {
		return nil, level, err
	}
	return resp.(*Response), level, err
}
