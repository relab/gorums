// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.7.0-devel
// 	protoc            v3.12.4
// source: broadcast/broadcast.proto

package broadcast

import (
	context "context"
	fmt "fmt"
	gorums "github.com/relab/gorums"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	encoding "google.golang.org/grpc/encoding"
	status "google.golang.org/grpc/status"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	net "net"
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
	nodes []*Node
	qspec QuorumSpec
	srv   *clientServerImpl
}

// ConfigurationFromRaw returns a new Configuration from the given raw configuration and QuorumSpec.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func ConfigurationFromRaw(rawCfg gorums.RawConfiguration, qspec QuorumSpec) *Configuration {
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	var test interface{} = struct{}{}
	if _, empty := test.(QuorumSpec); !empty && qspec == nil {
		panic("QuorumSpec may not be nil")
	}
	return &Configuration{
		RawConfiguration: rawCfg,
		qspec:            qspec,
	}
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *Configuration) Nodes() []*Node {
	if c.nodes == nil {
		c.nodes = make([]*Node, 0, c.Size())
		for _, n := range c.RawConfiguration {
			c.nodes = append(c.nodes, &Node{n})
		}
	}
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
func NewManager(opts ...gorums.ManagerOption) (mgr *Manager) {
	mgr = &Manager{}
	mgr.RawManager = gorums.NewRawManager(opts...)
	return mgr
}

// NewConfiguration returns a configuration based on the provided list of nodes (required)
// and an optional quorum specification. The QuorumSpec is necessary for call types that
// must process replies. For configurations only used for unicast or multicast call types,
// a QuorumSpec is not needed. The QuorumSpec interface is also a ConfigOption.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *Manager) NewConfiguration(opts ...gorums.ConfigOption) (c *Configuration, err error) {
	if len(opts) < 1 || len(opts) > 3 {
		return nil, fmt.Errorf("wrong number of options: %d", len(opts))
	}
	c = &Configuration{}
	for _, opt := range opts {
		switch v := opt.(type) {
		case gorums.NodeListOption:
			c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, v)
			if err != nil {
				return nil, err
			}
		case net.Listener:
			err = c.RegisterClientServer(v)
			if err != nil {
				return nil, err
			}
			return c, nil
		case QuorumSpec:
			// Must be last since v may match QuorumSpec if it is interface{}
			c.qspec = v
		default:
			return nil, fmt.Errorf("unknown option type: %v", v)
		}
	}
	// return an error if the QuorumSpec interface is not empty and no implementation was provided.
	//var test interface{} = struct{}{}
	//if _, empty := test.(QuorumSpec); !empty && c.qspec == nil {
	//	return nil, fmt.Errorf("missing required QuorumSpec")
	//}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *Manager) Nodes() []*Node {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*Node, 0, len(gorumsNodes))
	for _, n := range gorumsNodes {
		nodes = append(nodes, &Node{n})
	}
	return nodes
}

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	*gorums.RawNode
}

type Server struct {
	*gorums.Server
	broadcast *Broadcast
	View      *Configuration
}

func NewServer() *Server {
	srv := &Server{
		Server: gorums.NewServer(),
	}
	b := &Broadcast{
		orchestrator: gorums.NewBroadcastOrchestrator(srv.Server),
	}
	srv.broadcast = b
	srv.RegisterBroadcaster(newBroadcaster)
	return srv
}

func newBroadcaster(m gorums.BroadcastMetadata, o *gorums.BroadcastOrchestrator) gorums.Broadcaster {
	return &Broadcast{
		orchestrator: o,
		metadata:     m,
	}
}

func (srv *Server) SetView(config *Configuration) {
	srv.View = config
	srv.RegisterConfig(config.RawConfiguration)
}

type Broadcast struct {
	orchestrator *gorums.BroadcastOrchestrator
	metadata     gorums.BroadcastMetadata
}

// Returns a readonly struct of the metadata used in the broadcast.
//
// Note: Some of the data are equal across the cluster, such as BroadcastID.
// Other fields are local, such as SenderAddr.
func (b *Broadcast) GetMetadata() gorums.BroadcastMetadata {
	return b.metadata
}

type clientServerImpl struct {
	*gorums.ClientServer
	grpcServer *grpc.Server
}

func (c *Configuration) RegisterClientServer(lis net.Listener, opts ...grpc.ServerOption) error {
	srvImpl := &clientServerImpl{
		grpcServer: grpc.NewServer(opts...),
	}
	srv, err := gorums.NewClientServer(lis)
	if err != nil {
		return err
	}
	srvImpl.grpcServer.RegisterService(&clientServer_ServiceDesc, srvImpl)
	go srvImpl.grpcServer.Serve(lis)
	srvImpl.ClientServer = srv
	c.srv = srvImpl
	return nil
}

func (b *Broadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.orchestrator.SendToClientHandler(b.metadata.BroadcastID, resp, err)
}

func (srv *Server) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID string) {
	srv.RetToClient(resp, err, broadcastID)
}

func (b *Broadcast) Broadcast(req *Request, opts ...gorums.BroadcastOption) {
	if b.metadata.BroadcastID == "" {
		panic("broadcastID cannot be empty. Use srv.BroadcastBroadcast instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	go b.orchestrator.BroadcastHandler("broadcast.Broadcast.Broadcast", req, b.metadata.BroadcastID, options)
}

func _clientBroadcastCall(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Response)
	if err := dec(in); err != nil {
		return nil, err
	}
	return srv.(clientServer).clientBroadcastCall(ctx, in)
}

func (srv *clientServerImpl) clientBroadcastCall(ctx context.Context, resp *Response) (*Response, error) {
	err := srv.AddResponse(ctx, resp)
	return resp, err
}

func (c *Configuration) BroadcastCall(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("a client server is not defined. Use configuration.RegisterClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	doneChan, cd := c.srv.AddRequest(ctx, in, gorums.ConvertToType(c.qspec.BroadcastCallQF), "broadcast.Broadcast.BroadcastCall")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	response, ok := <-doneChan
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	return response.(*Response), err
}

// clientServer is the client server API for the Broadcast Service
type clientServer interface {
	clientBroadcastCall(ctx context.Context, request *Response) (*Response, error)
}

var clientServer_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "protos.ClientServer",
	HandlerType: (*clientServer)(nil),
	Methods: []grpc.MethodDesc{

		{
			MethodName: "ClientBroadcastCall",
			Handler:    _clientBroadcastCall,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "",
}

// QuorumSpec is the interface of quorum functions for Broadcast.
type QuorumSpec interface {
	gorums.ConfigOption

	// BroadcastCallQF is the quorum function for the BroadcastCall
	// broadcastcall call method. The in parameter is the request object
	// supplied to the BroadcastCall method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	BroadcastCallQF(replies []*Response) (*Response, bool)
}

// Broadcast is the server-side API for the Broadcast Service
type Broadcast interface {
	BroadcastCall(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	Broadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
}

func (srv *Server) BroadcastCall(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastCall not implemented"))
}
func (srv *Server) Broadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method Broadcast not implemented"))
}

func RegisterBroadcastServer(srv *Server, impl Broadcast) {
	srv.RegisterHandler("broadcast.Broadcast.BroadcastCall", gorums.BroadcastHandler(impl.BroadcastCall, srv.Server))
	srv.RegisterClientHandler("broadcast.Broadcast.BroadcastCall", gorums.ServerClientRPC("broadcast.Broadcast.BroadcastCall"))
	srv.RegisterHandler("broadcast.Broadcast.Broadcast", gorums.BroadcastHandler(impl.Broadcast, srv.Server))
}

func (srv *Server) BroadcastBroadcastCall(req *Request, broadcastID string, opts ...gorums.BroadcastOption) {
	if broadcastID == "" {
		panic("broadcastID cannot be empty.")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	go srv.broadcast.orchestrator.BroadcastHandler("broadcast.Broadcast.BroadcastCall", req, broadcastID, options)
}

func (srv *Server) BroadcastBroadcast(req *Request, broadcastID string, opts ...gorums.BroadcastOption) {
	if broadcastID == "" {
		panic("broadcastID cannot be empty.")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	go srv.broadcast.orchestrator.BroadcastHandler("broadcast.Broadcast.Broadcast", req, broadcastID, options)
}
