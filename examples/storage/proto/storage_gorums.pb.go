// Code generated by protoc-gen-gorums. DO NOT EDIT.

package proto

import (
	context "context"
	fmt "fmt"
	empty "github.com/golang/protobuf/ptypes/empty"
	gorums "github.com/relab/gorums"
	encoding "google.golang.org/grpc/encoding"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	sort "sort"
	sync "sync"
)

// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	id    uint32
	nodes []*gorums.Node
	n     int
	mgr   *Manager
	qspec QuorumSpec
	errs  chan gorums.Error
}

// NewConfig returns a configuration for the given node addresses and quorum spec.
// The returned func() must be called to close the underlying connections.
// This is an experimental API.
func NewConfig(qspec QuorumSpec, opts ...gorums.ManagerOption) (*Configuration, func(), error) {
	man, err := NewManager(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create manager: %v", err)
	}
	c, err := man.NewConfiguration(man.NodeIDs(), qspec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create configuration: %v", err)
	}
	return c, func() { man.Close() }, nil
}

// ID reports the identifier for the configuration.
func (c *Configuration) ID() uint32 {
	return c.id
}

// NodeIDs returns a slice containing the local ids of all the nodes in the
// configuration. IDs are returned in the same order as they were provided in
// the creation of the Configuration.
func (c *Configuration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c.nodes))
	for i, node := range c.nodes {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Configuration.
func (c *Configuration) Nodes() []*Node {
	nodes := make([]*Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		nodes = append(nodes, &Node{n, c.mgr})
	}
	return nodes
}

// Size returns the number of nodes in the configuration.
func (c *Configuration) Size() int {
	return c.n
}

func (c *Configuration) String() string {
	return fmt.Sprintf("config-%d", c.id)
}

// Equal returns a boolean reporting whether a and b represents the same
// configuration.
func Equal(a, b *Configuration) bool { return a.id == b.id }

// SubError returns a channel for listening to individual node errors. Currently
// only a single listener is supported.
func (c *Configuration) SubError() <-chan gorums.Error {
	return c.errs
}

func init() {
	encoding.RegisterCodec(gorums.NewGorumsCodec(orderingMethods))
}

func NewManager(opts ...gorums.ManagerOption) (mgr *Manager, err error) {
	mgr = &Manager{}
	mgr.Manager, err = gorums.NewManager(opts...)
	if err != nil {
		return nil, err
	}
	return mgr, nil
}

type Manager struct {
	*gorums.Manager
}

func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (*Configuration, error) {
	if len(ids) == 0 {
		return nil, gorums.IllegalConfigError("need at least one node")
	}

	var nodes []*gorums.Node
	unique := make(map[uint32]struct{})
	for _, nid := range ids {
		// ensure that identical IDs are only counted once
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}

		node, found := m.Node(nid)
		if !found {
			return nil, gorums.NodeNotFoundError(nid)
		}

		i := sort.Search(len(nodes), func(i int) bool {
			return node.ID() < nodes[i].ID()
		})
		nodes = append(nodes, nil)
		copy(nodes[i+1:], nodes[i:])
		nodes[i] = node
	}

	c := &Configuration{
		nodes: nodes,
		n:     len(nodes),
		mgr:   m,
		qspec: qspec,
	}
	return c, nil
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.Manager.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n, m})
	}
	return nodes
}

type Node struct {
	*gorums.Node
	mgr *Manager
}

// Reference imports to suppress errors if they are not otherwise used.
var _ empty.Empty

// WriteMulticast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) WriteMulticast(ctx context.Context, in *WriteRequest) {

	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: writeMulticastMethodID,
	}

	gorums.Multicast(ctx, cd)
}

// QuorumSpec is the interface of quorum functions for Storage.
type QuorumSpec interface {

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
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: readQCMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*ReadResponse, len(replies))
		for k, v := range replies {
			r[k] = v.(*ReadResponse)
		}
		return c.qspec.ReadQCQF(req.(*ReadRequest), r)
	}

	res, err := gorums.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*ReadResponse), err
}

// WriteQC executes the Write Quorum Call on a configuration
// of Nodes and returns true if a majority of Nodes were updated.
func (c *Configuration) WriteQC(ctx context.Context, in *WriteRequest) (resp *WriteResponse, err error) {

	cd := gorums.QuorumCallData{
		Manager:  c.mgr.Manager,
		Nodes:    c.nodes,
		Message:  in,
		MethodID: writeQCMethodID,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*WriteResponse, len(replies))
		for k, v := range replies {
			r[k] = v.(*WriteResponse)
		}
		return c.qspec.WriteQCQF(req.(*WriteRequest), r)
	}

	res, err := gorums.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*WriteResponse), err
}

// ReadRPC executes the Read RPC on a single Node
func (n *Node) ReadRPC(ctx context.Context, in *ReadRequest) (resp *ReadResponse, err error) {

	cd := gorums.CallData{
		Manager:  n.mgr.Manager,
		Node:     n.Node,
		Message:  in,
		MethodID: readRPCMethodID,
	}

	res, err := gorums.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*ReadResponse), err
}

// WriteRPC executes the Write RPC on a single Node
func (n *Node) WriteRPC(ctx context.Context, in *WriteRequest) (resp *WriteResponse, err error) {

	cd := gorums.CallData{
		Manager:  n.mgr.Manager,
		Node:     n.Node,
		Message:  in,
		MethodID: writeRPCMethodID,
	}

	res, err := gorums.RPCCall(ctx, cd)
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
	srv.RegisterHandler(readRPCMethodID, func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*ReadRequest)
		once := new(sync.Once)
		f := func(resp *ReadResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.ReadRPC(ctx, req, f)
	})
	srv.RegisterHandler(writeRPCMethodID, func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		once := new(sync.Once)
		f := func(resp *WriteResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.WriteRPC(ctx, req, f)
	})
	srv.RegisterHandler(readQCMethodID, func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*ReadRequest)
		once := new(sync.Once)
		f := func(resp *ReadResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.ReadQC(ctx, req, f)
	})
	srv.RegisterHandler(writeQCMethodID, func(ctx context.Context, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		once := new(sync.Once)
		f := func(resp *WriteResponse, err error) {
			once.Do(func() {
				finished <- gorums.WrapMessage(in.Metadata, resp, err)
			})
		}
		impl.WriteQC(ctx, req, f)
	})
	srv.RegisterHandler(writeMulticastMethodID, func(ctx context.Context, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*WriteRequest)
		impl.WriteMulticast(ctx, req)
	})
}

const readRPCMethodID int32 = 0
const writeRPCMethodID int32 = 1
const readQCMethodID int32 = 2
const writeQCMethodID int32 = 3
const writeMulticastMethodID int32 = 4

var orderingMethods = map[int32]gorums.MethodInfo{

	0: {RequestType: new(ReadRequest).ProtoReflect(), ResponseType: new(ReadResponse).ProtoReflect()},
	1: {RequestType: new(WriteRequest).ProtoReflect(), ResponseType: new(WriteResponse).ProtoReflect()},
	2: {RequestType: new(ReadRequest).ProtoReflect(), ResponseType: new(ReadResponse).ProtoReflect()},
	3: {RequestType: new(WriteRequest).ProtoReflect(), ResponseType: new(WriteResponse).ProtoReflect()},
	4: {RequestType: new(WriteRequest).ProtoReflect(), ResponseType: new(empty.Empty).ProtoReflect()},
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
