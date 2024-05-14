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
	qspec     QuorumSpec
	srv       *clientServerImpl
	snowflake gorums.Snowflake
	nodes     []*Node
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
	srv *clientServerImpl
}

// NewManager returns a new Manager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewManager(opts ...gorums.ManagerOption) *Manager {
	return &Manager{
		RawManager: gorums.NewRawManager(opts...),
	}
}

func (mgr *Manager) Close() {
	if mgr.RawManager != nil {
		mgr.RawManager.Close()
	}
	if mgr.srv != nil {
		mgr.srv.stop()
	}
}

func (mgr *Manager) AddClientServer(lis net.Listener, opts ...grpc.ServerOption) error {
	srv := gorums.NewClientServer(lis)
	srvImpl := &clientServerImpl{
		ClientServer: srv,
	}
	registerClientServerHandlers(srvImpl)
	go srvImpl.Serve(lis)
	mgr.srv = srvImpl
	return nil
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
	// register the client server if it exists.
	// used to collect responses in BroadcastCalls
	if m.srv != nil {
		c.srv = m.srv
	}
	c.snowflake = m.Snowflake()
	//var test interface{} = struct{}{}
	//if _, empty := test.(QuorumSpec); !empty && c.qspec == nil {
	//	return nil, fmt.Errorf("config: missing required QuorumSpec")
	//}
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

type Server struct {
	*gorums.Server
	broadcast *Broadcast
	View      *Configuration
}

func NewServer(opts ...gorums.ServerOption) *Server {
	srv := &Server{
		Server: gorums.NewServer(opts...),
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
		srvAddrs:     make([]string, 0),
	}
}

func (srv *Server) SetView(config *Configuration) {
	srv.View = config
	srv.RegisterConfig(config.RawConfiguration)
}

type Broadcast struct {
	orchestrator *gorums.BroadcastOrchestrator
	metadata     gorums.BroadcastMetadata
	srvAddrs     []string
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

func (c *clientServerImpl) stop() {
	c.ClientServer.Stop()
	if c.grpcServer != nil {
		c.grpcServer.Stop()
	}
}

func (b *Broadcast) To(addrs ...string) *Broadcast {
	if len(addrs) <= 0 {
		return b
	}
	b.srvAddrs = append(b.srvAddrs, addrs...)
	return b
}

func (b *Broadcast) Forward(req protoreflect.ProtoMessage, addr string) error {
	if addr == "" {
		return fmt.Errorf("cannot forward to empty addr, got: %s", addr)
	}
	if !b.metadata.IsBroadcastClient {
		return fmt.Errorf("can only forward client requests")
	}
	go b.orchestrator.ForwardHandler(req, b.metadata.OriginMethod, b.metadata.BroadcastID, addr, b.metadata.OriginAddr)
	return nil
}

// Done signals the end of a broadcast request. It is necessary to call
// either Done() or SendToClient() to properly terminate a broadcast request
// and free up resources. Otherwise, it could cause poor performance.
func (b *Broadcast) Done() {
	b.orchestrator.DoneHandler(b.metadata.BroadcastID)
}

// SendToClient sends a message back to the calling client. It also terminates
// the broadcast request, meaning subsequent messages related to the broadcast
// request will be dropped. Either SendToClient() or Done() should be used at
// the end of a broadcast request in order to free up resources.
func (b *Broadcast) SendToClient(resp protoreflect.ProtoMessage, err error) {
	b.orchestrator.SendToClientHandler(b.metadata.BroadcastID, resp, err)
}

// Cancel is a non-destructive method call that will transmit a cancellation
// to all servers in the view. It will not stop the execution but will cause
// the given ServerCtx to be cancelled, making it possible to listen for
// cancellations.
//
// Could be used together with either SendToClient() or Done().
func (b *Broadcast) Cancel() {
	b.orchestrator.CancelHandler(b.metadata.BroadcastID, b.srvAddrs)
}

// SendToClient sends a message back to the calling client. It also terminates
// the broadcast request, meaning subsequent messages related to the broadcast
// request will be dropped. Either SendToClient() or Done() should be used at
// the end of a broadcast request in order to free up resources.
func (srv *Server) SendToClient(resp protoreflect.ProtoMessage, err error, broadcastID uint64) {
	srv.SendToClientHandler(resp, err, broadcastID)
}

func (b *Broadcast) QuorumCallWithBroadcast(req *Request, opts ...gorums.BroadcastOption) {
	if b.metadata.BroadcastID == 0 {
		panic("broadcastID cannot be empty. Use srv.BroadcastQuorumCallWithBroadcast instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	options.ServerAddresses = append(options.ServerAddresses, b.srvAddrs...)
	b.orchestrator.BroadcastHandler("broadcast.BroadcastService.QuorumCallWithBroadcast", req, b.metadata.BroadcastID, options)
}

func (b *Broadcast) BroadcastIntermediate(req *Request, opts ...gorums.BroadcastOption) {
	if b.metadata.BroadcastID == 0 {
		panic("broadcastID cannot be empty. Use srv.BroadcastBroadcastIntermediate instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	options.ServerAddresses = append(options.ServerAddresses, b.srvAddrs...)
	b.orchestrator.BroadcastHandler("broadcast.BroadcastService.BroadcastIntermediate", req, b.metadata.BroadcastID, options)
}

func (b *Broadcast) Broadcast(req *Request, opts ...gorums.BroadcastOption) {
	if b.metadata.BroadcastID == 0 {
		panic("broadcastID cannot be empty. Use srv.BroadcastBroadcast instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	options.ServerAddresses = append(options.ServerAddresses, b.srvAddrs...)
	b.orchestrator.BroadcastHandler("broadcast.BroadcastService.Broadcast", req, b.metadata.BroadcastID, options)
}

func (b *Broadcast) BroadcastToResponse(req *Request, opts ...gorums.BroadcastOption) {
	if b.metadata.BroadcastID == 0 {
		panic("broadcastID cannot be empty. Use srv.BroadcastBroadcastToResponse instead")
	}
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	options.ServerAddresses = append(options.ServerAddresses, b.srvAddrs...)
	b.orchestrator.BroadcastHandler("broadcast.BroadcastService.BroadcastToResponse", req, b.metadata.BroadcastID, options)
}

func (srv *clientServerImpl) clientBroadcastCall(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) BroadcastCall(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.BroadcastCallQF), "broadcast.BroadcastService.BroadcastCall")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func (srv *clientServerImpl) clientBroadcastCallForward(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) BroadcastCallForward(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.BroadcastCallForwardQF), "broadcast.BroadcastService.BroadcastCallForward")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func (srv *clientServerImpl) clientBroadcastCallTo(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) BroadcastCallTo(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.BroadcastCallToQF), "broadcast.BroadcastService.BroadcastCallTo")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func (srv *clientServerImpl) clientSearch(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) Search(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.SearchQF), "broadcast.BroadcastService.Search")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func (srv *clientServerImpl) clientLongRunningTask(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) LongRunningTask(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.LongRunningTaskQF), "broadcast.BroadcastService.LongRunningTask")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func (srv *clientServerImpl) clientGetVal(ctx context.Context, resp *Response, broadcastID uint64) (*Response, error) {
	err := srv.AddResponse(ctx, resp, broadcastID)
	return resp, err
}

func (c *Configuration) GetVal(ctx context.Context, in *Request) (resp *Response, err error) {
	if c.srv == nil {
		return nil, fmt.Errorf("config: a client server is not defined. Use mgr.AddClientServer() to define a client server")
	}
	if c.qspec == nil {
		return nil, fmt.Errorf("a qspec is not defined")
	}
	broadcastID := c.snowflake.NewBroadcastID()
	doneChan, cd := c.srv.AddRequest(broadcastID, ctx, in, gorums.ConvertToType(c.qspec.GetValQF), "broadcast.BroadcastService.GetVal")
	c.RawConfiguration.Multicast(ctx, cd, gorums.WithNoSendWaiting())
	var response protoreflect.ProtoMessage
	var ok bool
	select {
	case response, ok = <-doneChan:
	case <-ctx.Done():
		bd := gorums.BroadcastCallData{
			Method:      gorums.Cancellation,
			BroadcastID: broadcastID,
		}
		c.RawConfiguration.BroadcastCall(context.Background(), bd)
		return nil, fmt.Errorf("context cancelled")
	}
	if !ok {
		return nil, fmt.Errorf("done channel was closed before returning a value")
	}
	resp, ok = response.(*Response)
	if !ok {
		return nil, fmt.Errorf("wrong proto format")
	}
	return resp, nil
}

func registerClientServerHandlers(srv *clientServerImpl) {

	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCall", gorums.ClientHandler(srv.clientBroadcastCall))
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCallForward", gorums.ClientHandler(srv.clientBroadcastCallForward))
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCallTo", gorums.ClientHandler(srv.clientBroadcastCallTo))
	srv.RegisterHandler("broadcast.BroadcastService.Search", gorums.ClientHandler(srv.clientSearch))
	srv.RegisterHandler("broadcast.BroadcastService.LongRunningTask", gorums.ClientHandler(srv.clientLongRunningTask))
	srv.RegisterHandler("broadcast.BroadcastService.GetVal", gorums.ClientHandler(srv.clientGetVal))
}

// Multicast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) Multicast(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "broadcast.BroadcastService.Multicast",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// MulticastIntermediate is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) MulticastIntermediate(ctx context.Context, in *Request, opts ...gorums.CallOption) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "broadcast.BroadcastService.MulticastIntermediate",
	}

	c.RawConfiguration.Multicast(ctx, cd, opts...)
}

// QuorumSpec is the interface of quorum functions for BroadcastService.
type QuorumSpec interface {
	gorums.ConfigOption

	// QuorumCallQF is the quorum function for the QuorumCall
	// quorum call method. The in parameter is the request object
	// supplied to the QuorumCall method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	QuorumCallQF(in *Request, replies map[uint32]*Response) (*Response, bool)

	// QuorumCallWithBroadcastQF is the quorum function for the QuorumCallWithBroadcast
	// quorum call method. The in parameter is the request object
	// supplied to the QuorumCallWithBroadcast method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	QuorumCallWithBroadcastQF(in *Request, replies map[uint32]*Response) (*Response, bool)

	// QuorumCallWithMulticastQF is the quorum function for the QuorumCallWithMulticast
	// quorum call method. The in parameter is the request object
	// supplied to the QuorumCallWithMulticast method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	QuorumCallWithMulticastQF(in *Request, replies map[uint32]*Response) (*Response, bool)

	// BroadcastCallQF is the quorum function for the BroadcastCall
	// broadcastcall call method. The in parameter is the request object
	// supplied to the BroadcastCall method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	BroadcastCallQF(in *Request, replies []*Response) (*Response, bool)

	// BroadcastCallForwardQF is the quorum function for the BroadcastCallForward
	// broadcastcall call method. The in parameter is the request object
	// supplied to the BroadcastCallForward method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	BroadcastCallForwardQF(in *Request, replies []*Response) (*Response, bool)

	// BroadcastCallToQF is the quorum function for the BroadcastCallTo
	// broadcastcall call method. The in parameter is the request object
	// supplied to the BroadcastCallTo method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	BroadcastCallToQF(in *Request, replies []*Response) (*Response, bool)

	// SearchQF is the quorum function for the Search
	// broadcastcall call method. The in parameter is the request object
	// supplied to the Search method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	SearchQF(in *Request, replies []*Response) (*Response, bool)

	// LongRunningTaskQF is the quorum function for the LongRunningTask
	// broadcastcall call method. The in parameter is the request object
	// supplied to the LongRunningTask method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	LongRunningTaskQF(in *Request, replies []*Response) (*Response, bool)

	// GetValQF is the quorum function for the GetVal
	// broadcastcall call method. The in parameter is the request object
	// supplied to the GetVal method at call time, and may or may not
	// be used by the quorum function. If the in parameter is not needed
	// you should implement your quorum function with '_ *Request'.
	GetValQF(in *Request, replies []*Response) (*Response, bool)
}

// QuorumCall is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) QuorumCall(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "broadcast.BroadcastService.QuorumCall",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// QuorumCallWithBroadcast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) QuorumCallWithBroadcast(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "broadcast.BroadcastService.QuorumCallWithBroadcast",

		BroadcastID:       c.snowflake.NewBroadcastID(),
		IsBroadcastClient: true,
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallWithBroadcastQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// QuorumCallWithMulticast is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (c *Configuration) QuorumCallWithMulticast(ctx context.Context, in *Request) (resp *Response, err error) {
	cd := gorums.QuorumCallData{
		Message: in,
		Method:  "broadcast.BroadcastService.QuorumCallWithMulticast",
	}
	cd.QuorumFunction = func(req protoreflect.ProtoMessage, replies map[uint32]protoreflect.ProtoMessage) (protoreflect.ProtoMessage, bool) {
		r := make(map[uint32]*Response, len(replies))
		for k, v := range replies {
			r[k] = v.(*Response)
		}
		return c.qspec.QuorumCallWithMulticastQF(req.(*Request), r)
	}

	res, err := c.RawConfiguration.QuorumCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Response), err
}

// BroadcastService is the server-side API for the BroadcastService Service
type BroadcastService interface {
	QuorumCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	QuorumCallWithBroadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	QuorumCallWithMulticast(ctx gorums.ServerCtx, request *Request) (response *Response, err error)
	Multicast(ctx gorums.ServerCtx, request *Request)
	MulticastIntermediate(ctx gorums.ServerCtx, request *Request)
	BroadcastCall(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	BroadcastIntermediate(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	Broadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	BroadcastCallForward(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	BroadcastCallTo(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	BroadcastToResponse(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	Search(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	LongRunningTask(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
	GetVal(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast)
}

func (srv *Server) QuorumCall(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic(status.Errorf(codes.Unimplemented, "method QuorumCall not implemented"))
}
func (srv *Server) QuorumCallWithBroadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method QuorumCallWithBroadcast not implemented"))
}
func (srv *Server) QuorumCallWithMulticast(ctx gorums.ServerCtx, request *Request) (response *Response, err error) {
	panic(status.Errorf(codes.Unimplemented, "method QuorumCallWithMulticast not implemented"))
}
func (srv *Server) Multicast(ctx gorums.ServerCtx, request *Request) {
	panic(status.Errorf(codes.Unimplemented, "method Multicast not implemented"))
}
func (srv *Server) MulticastIntermediate(ctx gorums.ServerCtx, request *Request) {
	panic(status.Errorf(codes.Unimplemented, "method MulticastIntermediate not implemented"))
}
func (srv *Server) BroadcastCall(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastCall not implemented"))
}
func (srv *Server) BroadcastIntermediate(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastIntermediate not implemented"))
}
func (srv *Server) Broadcast(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method Broadcast not implemented"))
}
func (srv *Server) BroadcastCallForward(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastCallForward not implemented"))
}
func (srv *Server) BroadcastCallTo(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastCallTo not implemented"))
}
func (srv *Server) BroadcastToResponse(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method BroadcastToResponse not implemented"))
}
func (srv *Server) Search(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method Search not implemented"))
}
func (srv *Server) LongRunningTask(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method LongRunningTask not implemented"))
}
func (srv *Server) GetVal(ctx gorums.ServerCtx, request *Request, broadcast *Broadcast) {
	panic(status.Errorf(codes.Unimplemented, "method GetVal not implemented"))
}

func RegisterBroadcastServiceServer(srv *Server, impl BroadcastService) {
	srv.RegisterHandler("broadcast.BroadcastService.QuorumCall", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCall(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("broadcast.BroadcastService.QuorumCallWithBroadcast", gorums.BroadcastHandler(impl.QuorumCallWithBroadcast, srv.Server))
	srv.RegisterHandler("broadcast.BroadcastService.QuorumCallWithMulticast", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		resp, err := impl.QuorumCallWithMulticast(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("broadcast.BroadcastService.Multicast", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.Multicast(ctx, req)
	})
	srv.RegisterHandler("broadcast.BroadcastService.MulticastIntermediate", func(ctx gorums.ServerCtx, in *gorums.Message, _ chan<- *gorums.Message) {
		req := in.Message.(*Request)
		defer ctx.Release()
		impl.MulticastIntermediate(ctx, req)
	})
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCall", gorums.BroadcastHandler(impl.BroadcastCall, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.BroadcastCall")
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastIntermediate", gorums.BroadcastHandler(impl.BroadcastIntermediate, srv.Server))
	srv.RegisterHandler("broadcast.BroadcastService.Broadcast", gorums.BroadcastHandler(impl.Broadcast, srv.Server))
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCallForward", gorums.BroadcastHandler(impl.BroadcastCallForward, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.BroadcastCallForward")
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastCallTo", gorums.BroadcastHandler(impl.BroadcastCallTo, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.BroadcastCallTo")
	srv.RegisterHandler("broadcast.BroadcastService.BroadcastToResponse", gorums.BroadcastHandler(impl.BroadcastToResponse, srv.Server))
	srv.RegisterHandler("broadcast.BroadcastService.Search", gorums.BroadcastHandler(impl.Search, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.Search")
	srv.RegisterHandler("broadcast.BroadcastService.LongRunningTask", gorums.BroadcastHandler(impl.LongRunningTask, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.LongRunningTask")
	srv.RegisterHandler("broadcast.BroadcastService.GetVal", gorums.BroadcastHandler(impl.GetVal, srv.Server))
	srv.RegisterClientHandler("broadcast.BroadcastService.GetVal")
	srv.RegisterHandler(gorums.Cancellation, gorums.BroadcastHandler(gorums.CancelFunc, srv.Server))
}

func (srv *Server) BroadcastQuorumCallWithBroadcast(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.RelatedToReq > 0 {
		srv.broadcast.orchestrator.BroadcastHandler("broadcast.BroadcastService.QuorumCallWithBroadcast", req, options.RelatedToReq, options)
	} else {
		srv.broadcast.orchestrator.ServerBroadcastHandler("broadcast.BroadcastService.QuorumCallWithBroadcast", req, options)
	}
}

func (srv *Server) BroadcastBroadcastIntermediate(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.RelatedToReq > 0 {
		srv.broadcast.orchestrator.BroadcastHandler("broadcast.BroadcastService.BroadcastIntermediate", req, options.RelatedToReq, options)
	} else {
		srv.broadcast.orchestrator.ServerBroadcastHandler("broadcast.BroadcastService.BroadcastIntermediate", req, options)
	}
}

func (srv *Server) BroadcastBroadcast(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.RelatedToReq > 0 {
		srv.broadcast.orchestrator.BroadcastHandler("broadcast.BroadcastService.Broadcast", req, options.RelatedToReq, options)
	} else {
		srv.broadcast.orchestrator.ServerBroadcastHandler("broadcast.BroadcastService.Broadcast", req, options)
	}
}

func (srv *Server) BroadcastBroadcastToResponse(req *Request, opts ...gorums.BroadcastOption) {
	options := gorums.NewBroadcastOptions()
	for _, opt := range opts {
		opt(&options)
	}
	if options.RelatedToReq > 0 {
		srv.broadcast.orchestrator.BroadcastHandler("broadcast.BroadcastService.BroadcastToResponse", req, options.RelatedToReq, options)
	} else {
		srv.broadcast.orchestrator.ServerBroadcastHandler("broadcast.BroadcastService.BroadcastToResponse", req, options)
	}
}

const (
	QuorumCallWithBroadcast string = "broadcast.BroadcastService.QuorumCallWithBroadcast"
	BroadcastCall           string = "broadcast.BroadcastService.BroadcastCall"
	//Broadcast           string = "broadcast.BroadcastService.Broadcast"
	BroadcastIntermediate   string = "broadcast.BroadcastService.BroadcastIntermediate"
	BroadcastCallForward    string = "broadcast.BroadcastService.BroadcastCallForward"
	BroadcastCallTo         string = "broadcast.BroadcastService.BroadcastCallTo"
	BroadcastToResponse     string = "broadcast.BroadcastService.BroadcastToResponse"
	Search                  string = "broadcast.BroadcastService.Search"
	LongRunningTask         string = "broadcast.BroadcastService.LongRunningTask"
	GetVal                  string = "broadcast.BroadcastService.GetVal"
)

type internalResponse struct {
	nid   uint32
	reply *Response
	err   error
}
