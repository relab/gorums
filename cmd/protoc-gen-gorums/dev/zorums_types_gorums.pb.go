// Code generated by protoc-gen-gorums. DO NOT EDIT.

package dev

import (
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const gRPCCallMethodID int32 = 0
const quorumCallMethodID int32 = 1
const quorumCallPerNodeArgMethodID int32 = 2
const quorumCallCustomReturnTypeMethodID int32 = 3
const quorumCallComboMethodID int32 = 4
const quorumCallEmptyMethodID int32 = 5
const quorumCallEmpty2MethodID int32 = 6
const multicastMethodID int32 = 7
const multicastPerNodeArgMethodID int32 = 8
const multicast2MethodID int32 = 9
const multicast3MethodID int32 = 10
const multicast4MethodID int32 = 11
const quorumCallFutureMethodID int32 = 12
const quorumCallFuturePerNodeArgMethodID int32 = 13
const quorumCallFutureCustomReturnTypeMethodID int32 = 14
const quorumCallFutureComboMethodID int32 = 15
const quorumCallFuture2MethodID int32 = 16
const quorumCallFutureEmptyMethodID int32 = 17
const quorumCallFutureEmpty2MethodID int32 = 18
const correctableMethodID int32 = 19
const correctablePerNodeArgMethodID int32 = 20
const correctableCustomReturnTypeMethodID int32 = 21
const correctableComboMethodID int32 = 22
const correctableEmptyMethodID int32 = 23
const correctableEmpty2MethodID int32 = 24
const correctableStreamMethodID int32 = 25
const correctableStreamPerNodeArgMethodID int32 = 26
const correctableStreamCustomReturnTypeMethodID int32 = 27
const correctableStreamComboMethodID int32 = 28
const correctableStreamEmptyMethodID int32 = 29
const correctableStreamEmpty2MethodID int32 = 30
const unicastMethodID int32 = 31
const unicast2MethodID int32 = 32

var orderingMethods = map[int32]gorums.MethodInfo{

	0:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	1:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	2:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	3:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	4:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	5:  {RequestType: new(emptypb.Empty).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	6:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	7:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	8:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	9:  {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	10: {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	11: {RequestType: new(emptypb.Empty).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	12: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	13: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	14: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	15: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	16: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	17: {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	18: {RequestType: new(emptypb.Empty).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	19: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	20: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	21: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	22: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	23: {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	24: {RequestType: new(emptypb.Empty).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	25: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	26: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	27: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	28: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	29: {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
	30: {RequestType: new(emptypb.Empty).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	31: {RequestType: new(Request).ProtoReflect(), ResponseType: new(Response).ProtoReflect()},
	32: {RequestType: new(Request).ProtoReflect(), ResponseType: new(emptypb.Empty).ProtoReflect()},
}

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

// FutureEmpty is a future object for processing replies.
type FutureEmpty struct {
	*gorums.Future
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureEmpty) Get() (*emptypb.Empty, error) {
	resp, err := f.Future.Get()
	if err != nil {
		return nil, err
	}
	return resp.(*emptypb.Empty), err
}

// FutureMyResponse is a future object for processing replies.
type FutureMyResponse struct {
	*gorums.Future
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureMyResponse) Get() (*MyResponse, error) {
	resp, err := f.Future.Get()
	if err != nil {
		return nil, err
	}
	return resp.(*MyResponse), err
}

// FutureResponse is a future object for processing replies.
type FutureResponse struct {
	*gorums.Future
}

// Get returns the reply and any error associated with the called method.
// The method blocks until a reply or error is available.
func (f *FutureResponse) Get() (*Response, error) {
	resp, err := f.Future.Get()
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
