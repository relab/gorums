// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.3.0-devel
// 	protoc            v3.15.6
// source: storage/proto/storage.proto

package proto

import (
	context "context"
	fmt "fmt"
	gorums "github.com/relab/gorums"
	encoding "google.golang.org/grpc/encoding"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(3 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 3)
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	gorums.Configuration
	qspec QuorumSpec
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (c *Configuration) Nodes() []*Node {
	nodes := make([]*Node, 0, c.Size())
	for _, n := range c.Configuration {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

// Add returns a NodeListOption that can be used to create a new configuration combining c and add.
func (c Configuration) Add(add *Configuration) gorums.NodeListOption {
	return c.Configuration.Add(add.Configuration)
}

// Remove returns a NodeListOption that can be used to create a new configuration
// from configuration c without the nodes in configuration rm.
func (c Configuration) Remove(rm *Configuration) gorums.NodeListOption {
	return c.Configuration.Remove(rm.Configuration)
}

func init() {
	if encoding.GetCodec(gorums.ContentSubtype) == nil {
		encoding.RegisterCodec(gorums.NewCodec())
	}
}

// Manager maintains a connection pool of nodes on
// which quorum calls can be performed.
type Manager struct {
	*gorums.Manager
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...gorums.ManagerOption) (mgr *Manager) {
	mgr = &Manager{}
	mgr.Manager = gorums.NewManager(opts...)
	return mgr
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed. The QuorumSpec interface is also a ConfigOption.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the Add, AddNodes, Remove, and RemoveNodes methods.
func (m *Manager) NewConfiguration(opts ...gorums.ConfigOption) (c *Configuration, err error) {
	if len(opts) < 1 || len(opts) > 2 {
		return nil, fmt.Errorf("wrong number of options: %d", len(opts))
	}
	c = &Configuration{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gorums.NodeListOption:
			c.Configuration, err = gorums.NewConfiguration(m.Manager, v)
			if err != nil {
				return nil, err
			}
		case QuorumSpec:
			// Must be last since v may match QuorumSpec if it is interface{}
			c.qspec = v
		default:
			return nil, fmt.Errorf("unknown option type: %v", v)
		}
	}
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && c.qspec == nil {
		return nil, fmt.Errorf("missing required QuorumSpec")
	}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.Manager.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

type Node struct {
	*gorums.Node
}

// Reference imports to suppress errors if they are not otherwise used.
var _ emptypb.Empty

// WriteMulticast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) WriteMulticast(ctx context.Context, in *WriteRequest, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "storage.Storage.WriteMulticast",
	}

	c.Configuration.Multicast(ctx, cd, opts...)
}

// QuorumSpec is the interface of quorum functions for Storage.
type QuorumSpec interface {
	gorums.ConfigOption

	// ReadQCQF is the quorum function for the ReadQC
	// quorum call method. The in parameter is the request object
	// supplied to the ReadQC method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *ReadRequest'.
	ReadQCQF(in *ReadRequest, replies map[uint32]*ReadResponse) (*ReadResponse, bool)

	// WriteQCQF is the quorum function for the WriteQC
	// quorum call method. The in parameter is the request object
	// supplied to the WriteQC method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *WriteRequest'.
	WriteQCQF(in *WriteRequest, replies map[uint32]*WriteResponse) (*WriteResponse, bool)
}

// ReadQC executes the Read Quorum Call on a configuration
// of Nodes and returns the most recent value.
func (c *Configuration) ReadQC(ctx context.Context, in *ReadRequest) (resp *ReadResponse, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "storage.Storage.ReadQC",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*ReadResponse, len(replies))
		for k, v := range replies {
			r[k] = v.(*ReadResponse)
		}
		return c.qspec.ReadQCQF(req.(*ReadRequest), r)
	}

	res, err := c.Configuration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*ReadResponse), err
}

// WriteQC executes the Write Quorum Call on a configuration
// of Nodes and returns true if a majority of Nodes were updated.
func (c *Configuration) WriteQC(ctx context.Context, in *WriteRequest) (resp *WriteResponse, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "storage.Storage.WriteQC",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*WriteResponse, len(replies))
		for k, v := range replies {
			r[k] = v.(*WriteResponse)
		}
		return c.qspec.WriteQCQF(req.(*WriteRequest), r)
	}

	res, err := c.Configuration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*WriteResponse), err
}

// ReadRPC executes the Read RPC on a single Node
func (n *Node) ReadRPC(ctx context.Context, in *ReadRequest) (resp *ReadResponse, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "storage.Storage.ReadRPC",
	}

	res, err := n.Node.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*ReadResponse), err
}

// WriteRPC executes the Write RPC on a single Node
func (n *Node) WriteRPC(ctx context.Context, in *WriteRequest) (resp *WriteResponse, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "storage.Storage.WriteRPC",
	}

	res, err := n.Node.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*WriteResponse), err
}

// Storage is the server-side API for the Storage Service
type Storage interface {
	ReadRPC(context.Context, *ReadRequest, func(*ReadResponse, error))
	WriteRPC(context.Context, *WriteRequest, func(*WriteResponse, error))
	ReadQC(context.Context, *ReadRequest, func(*ReadResponse, error))
	WriteQC(context.Context, *WriteRequest, func(*WriteResponse, error))
	WriteMulticast(context.Context, *WriteRequest)
}

func RegisterStorageServer(srv *gorums.Server, impl Storage) {
	srv.RegisterHandler("storage.Storage.ReadRPC", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*ReadRequest)
		once := new(sync.Once)
		f := func(resp *ReadResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.ReadRPC(ctx, req, f)
	})
	srv.RegisterHandler("storage.Storage.WriteRPC", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		once := new(sync.Once)
		f := func(resp *WriteResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.WriteRPC(ctx, req, f)
	})
	srv.RegisterHandler("storage.Storage.ReadQC", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*ReadRequest)
		once := new(sync.Once)
		f := func(resp *ReadResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.ReadQC(ctx, req, f)
	})
	srv.RegisterHandler("storage.Storage.WriteQC", func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		once := new(sync.Once)
		f := func(resp *WriteResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.WriteQC(ctx, req, f)
	})
	srv.RegisterHandler("storage.Storage.WriteMulticast", func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		impl.WriteMulticast(ctx, req)
	})
}

type internalReadResponse struct {
	nid   uint32
	reply *ReadResponse
	err   error
}

type internalWriteResponse struct {
	nid   uint32
	reply *WriteResponse
	err   error
}
