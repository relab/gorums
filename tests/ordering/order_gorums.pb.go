// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.8.0-devel
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
	_ = gorums.EnforceVersion(8 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 8)
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.RawConfiguration
	qspec QuorumSpec
	nodes []*Node
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
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
	for i, n := range rawCfg {
		newCfg.nodes[i] = &Node{n}
	}
	return newCfg, nil
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

// Manager maintains a connection pool of nodes on
// which quorum calls can be performed.
type Manager struct {
	*gorums.RawManager
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...gorums.ManagerOption) *Manager {
	return &Manager{
		RawManager: gorums.NewRawManager(opts...),
	}
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed. The QuorumSpec interface is also a ConfigOption.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *Manager) NewConfiguration(opts ...gorums.ConfigOption) (c *Configuration, err error) {
	if len(opts) < 1 || len(opts) > 2 {
		return nil, fmt.Errorf("config: wrong number of options: %d", len(opts))
	}
	c = &Configuration{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gorums.NodeListOption:
			c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, v)
			if err != nil {
				return nil, err
			}
		case QuorumSpec:
			// Must be last since v may match QuorumSpec if it is interface{}
			c.qspec = v
		default:
			return nil, fmt.Errorf("config: unknown option type: %v", v)
		}
	}
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && c.qspec == nil {
		return nil, fmt.Errorf("config: missing required QuorumSpec")
	}
	// initialize the nodes slice
	c.nodes = make([]*Node, c.Size())
	for i, n := range c.RawConfiguration {
		c.nodes[i] = &Node{n}
	}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*Node, len(gorumsNodes))
	for i, n := range gorumsNodes {
		nodes[i] = &Node{n}
	}
	return nodes
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
var _ GorumsTestNodeClient = (*GorumsTestNode)(nil)

// GorumsTestNode holds the node specific methods for the GorumsTest service.
type GorumsTestNode struct {
	*gorums.RawNode
}

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
func (n *GorumsTestNode) UnaryRPC(ctx context.Context, in *Request) (resp *Response, err error) {
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
