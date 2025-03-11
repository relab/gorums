// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v5.29.2
// source: ordering/order.proto

package ordering

import (
	context "context"
	fmt "fmt"
	gorums "github.com/relab/gorums"
	encoding "google.golang.org/grpc/encoding"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(7 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 7)
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.RawConfiguration
	qspec QuorumSpec
	nodes []*Node
}

func NewConfiguration(qspec QuorumSpec, cfg gorums.NodeListOption, opts ...gorums.ManagerOption) (c *Configuration, err error) {
	if qspec == nil {
		return nil, fmt.Errorf("config: QuorumSpec cannot be nil")
	}
	c = &Configuration{
		qspec: qspec,
	}
	c.RawConfiguration, err = gorums.NewRawConfiguration(cfg, opts...)
	if err != nil {
		return nil, err
	}
	c.nodes = make([]*Node, c.Size())
	for i, n := range c.RawConfiguration.RawNodes {
		c.nodes[i] = &Node{n}
	}
	return c, nil
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfiguration, qspec2)
func ConfigurationFromRaw(rawCfg gorums.RawConfiguration, qspec QuorumSpec) (*Configuration, error) {
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && qspec == nil {
		return nil, fmt.Errorf("config: missing required QuorumSpec")
	}
	newCfg := &Configuration{
		RawConfiguration: rawCfg,
		qspec:            qspec,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*Node, newCfg.Size())
	for i, n := range rawCfg.Nodes() {
		newCfg.nodes[i] = &Node{n}
	}
	return newCfg, nil
}

func (c *Configuration) SubConfiguration(qspec QuorumSpec, cfg gorums.NodeListOption) (subCfg *Configuration, err error) {
	if qspec == nil {
		return nil, fmt.Errorf("config: QuorumSpec cannot be nil")
	}
	subCfg = &Configuration{
		qspec: qspec,
	}
	subCfg.RawConfiguration, err = c.SubRawConfiguration(cfg)
	if err != nil {
		return nil, err
	}
	subCfg.nodes = make([]*Node, subCfg.Size())
	for i, n := range subCfg.RawConfiguration.Nodes() {
		subCfg.nodes[i] = &Node{n}
	}
	return subCfg, nil
}

// Close closes a configuration created from the NewConfiguration method
//
// NOTE: A configuration created with ConfigurationFromRaw is closed when the original configuration is closed
// If you want the configurations to be independent you need to use NewConfiguration
func (c *Configuration) Close() error {
	return c.RawConfiguration.Close()
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration) Nodes() []*Node {
	return c.nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c Configuration) And(d *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c Configuration) Except(rm *Configuration) gorums.NodeListOption {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	*gorums.RawNode
}

// QCAsync asynchronously invokes a quorum call on configuration c
// and returns a AsyncResponse, which can be used to inspect the quorum call
// reply and error when available.
func (c *Configuration) QCAsync(ctx context.Context, in *Request) *AsyncResponse {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "ordering.GorumsTest.QCAsync",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QCAsyncQF(req.(*Request), r)
	}

	fut := c.RawConfiguration.AsyncCall(ctx, cd)
	return &AsyncResponse{fut}
}

// GorumsTestClient is the client interface for the GorumsTest service.
type GorumsTestClient interface {
	QC(ctx context.Context, in *Request) (resp *Response, err error)
	QCAsync(ctx context.Context, in *Request) *AsyncResponse
}

// enforce interface compliance
var _ GorumsTestClient = (*Configuration)(nil)

// GorumsTestNodeClient is the single node client interface for the GorumsTest service.
type GorumsTestNodeClient interface {
	UnaryRPC(ctx context.Context, in *Request) (resp *Response, err error)
}

// enforce interface compliance
var _ GorumsTestNodeClient = (*Node)(nil)

// QuorumSpec is the interface of quorum functions for GorumsTest.
type QuorumSpec interface {
	gorums.ConfigOption

	// QCQF is the quorum function for the QC
	// quorum call method. The in parameter is the request object
	// supplied to the QC method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	QCQF(in *Request, replies map[uint32]*Response) (*Response, bool)

	// QCAsyncQF is the quorum function for the QCAsync
	// asynchronous quorum call method. The in parameter is the request object
	// supplied to the QCAsync method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	QCAsyncQF(in *Request, replies map[uint32]*Response) (*Response, bool)
}

// QC is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) QC(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "ordering.GorumsTest.QC",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QCQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// UnaryRPC is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (n *Node) UnaryRPC(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "ordering.GorumsTest.UnaryRPC",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// GorumsTest is the server-side API for the GorumsTest Service
type GorumsTestServer interface {
	QC(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QCAsync(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	UnaryRPC(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
}

func RegisterGorumsTestServer(srv *gorums.Server, impl GorumsTestServer) {
	srv.RegisterHandler("ordering.GorumsTest.QC", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QC(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("ordering.GorumsTest.QCAsync", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QCAsync(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("ordering.GorumsTest.UnaryRPC", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.UnaryRPC(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
}

type internalResponse struct {
	nid   uint32
	reply *Response
	err   error
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
