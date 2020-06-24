// Code generated by protoc-gen-gorums. DO NOT EDIT.
// Source files can be found in: ./cmd/protoc-gen-gorums/dev

package gengorums

// pkgIdentMap maps from package name to one of the package's identifiers.
// These identifiers are used by the Gorums protoc plugin to generate
// appropriate import statements.
var pkgIdentMap = map[string]string{
	"bytes":                            "Buffer",
	"context":                          "Background",
	"encoding/binary":                  "LittleEndian",
	"fmt":                              "Errorf",
	"github.com/relab/gorums/ordering": "GorumsClient",
	"golang.org/x/net/trace":           "EventLog",
	"google.golang.org/grpc":           "CallContentSubtype",
	"google.golang.org/grpc/backoff":   "Config",
	"google.golang.org/grpc/codes":     "Unavailable",
	"google.golang.org/grpc/encoding":  "RegisterCodec",
	"google.golang.org/grpc/status":    "Errorf",
	"google.golang.org/protobuf/encoding/protowire":   "AppendVarint",
	"google.golang.org/protobuf/proto":                "MarshalOptions",
	"google.golang.org/protobuf/reflect/protoreflect": "Message",
	"hash/fnv":    "New32a",
	"io":          "WriteString",
	"log":         "Logger",
	"math":        "Min",
	"math/rand":   "Float64",
	"net":         "Listener",
	"sort":        "Sort",
	"strconv":     "Atoi",
	"strings":     "Join",
	"sync":        "Mutex",
	"sync/atomic": "AddUint64",
	"time":        "After",
}

// reservedIdents holds the set of Gorums reserved identifiers.
// These identifiers cannot be used to define message types in a proto file.
var reservedIdents = []string{
	"ConfigNotFoundError",
	"Configuration",
	"GRPCError",
	"GorumsServer",
	"IllegalConfigError",
	"Manager",
	"ManagerCreationError",
	"ManagerOption",
	"MultiSorter",
	"NewManager",
	"Node",
	"NodeNotFoundError",
	"QuorumCallError",
	"QuorumSpec",
	"ServerOption",
}

var staticCode = `// A Configuration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type Configuration struct {
	id    uint32
	nodes []*Node
	n     int
	mgr   *Manager
	qspec QuorumSpec
	errs  chan GRPCError
}

// NewConfig returns a configuration for the given node addresses and quorum spec.
// The returned func() must be called to close the underlying connections.
// This is experimental API.
func NewConfig(addrs []string, qspec QuorumSpec, opts ...ManagerOption) (*Configuration, func(), error) {
	man, err := NewManager(addrs, opts...)
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
	return c.nodes
}

// Size returns the number of nodes in the configuration.
func (c *Configuration) Size() int {
	return c.n
}

func (c *Configuration) String() string {
	return fmt.Sprintf("configuration %d", c.id)
}

func (c *Configuration) tstring() string {
	return fmt.Sprintf("config-%d", c.id)
}

// Equal returns a boolean reporting whether a and b represents the same
// configuration.
func Equal(a, b *Configuration) bool { return a.id == b.id }

// SubError returns a channel for listening to individual node errors. Currently
// only a single listener is supported.
func (c *Configuration) SubError() <-chan GRPCError {
	return c.errs
}

const gorumsContentType = "gorums"

func init() {
	encoding.RegisterCodec(newGorumsCodec())
}

type gorumsMessage struct {
	metadata *ordering.Metadata
	message  protoreflect.ProtoMessage
	reply    bool
}

func newGorumsMessage(reply bool) *gorumsMessage {
	return &gorumsMessage{metadata: &ordering.Metadata{}, reply: reply}
}

type gorumsCodec struct {
	marshaler   proto.MarshalOptions
	unmarshaler proto.UnmarshalOptions
}

func newGorumsCodec() *gorumsCodec {
	return &gorumsCodec{
		marshaler:   proto.MarshalOptions{AllowPartial: true},
		unmarshaler: proto.UnmarshalOptions{AllowPartial: true},
	}
}

func (c gorumsCodec) Name() string {
	return gorumsContentType
}

func (c gorumsCodec) String() string {
	return gorumsContentType
}

func (c gorumsCodec) Marshal(m interface{}) (b []byte, err error) {
	switch msg := m.(type) {
	case *gorumsMessage:
		return c.gorumsMarshal(msg)
	case protoreflect.ProtoMessage:
		return c.marshaler.Marshal(msg)
	default:
		return nil, fmt.Errorf("gorumsEncoder: don't know how to marshal message of type '%T'", m)
	}
}

// gorumsMarshal marshals a metadata and a data message into a single byte slice.
func (c gorumsCodec) gorumsMarshal(msg *gorumsMessage) (b []byte, err error) {
	mdSize := c.marshaler.Size(msg.metadata)
	b = protowire.AppendVarint(b, uint64(mdSize))
	b, err = c.marshaler.MarshalAppend(b, msg.metadata)
	if err != nil {
		return nil, err
	}

	msgSize := c.marshaler.Size(msg.message)
	b = protowire.AppendVarint(b, uint64(msgSize))
	b, err = c.marshaler.MarshalAppend(b, msg.message)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (c gorumsCodec) Unmarshal(b []byte, m interface{}) (err error) {
	switch msg := m.(type) {
	case *gorumsMessage:
		return c.gorumsUnmarshal(b, msg)
	case protoreflect.ProtoMessage:
		return c.unmarshaler.Unmarshal(b, msg)
	default:
		return fmt.Errorf("gorumsEncoder: don't know how to unmarshal message of type '%T'", m)
	}
}

// gorumsUnmarshal unmarshals a metadata and a data message from a byte slice.
func (c gorumsCodec) gorumsUnmarshal(b []byte, msg *gorumsMessage) (err error) {
	mdBuf, mdLen := protowire.ConsumeBytes(b)
	err = c.unmarshaler.Unmarshal(mdBuf, msg.metadata)
	if err != nil {
		return err
	}
	info := orderingMethods[msg.metadata.MethodID]
	if msg.reply {
		msg.message = info.responseType.New().Interface()
	} else {
		msg.message = info.requestType.New().Interface()
	}
	msgBuf, _ := protowire.ConsumeBytes(b[mdLen:])
	err = c.unmarshaler.Unmarshal(msgBuf, msg.message)
	if err != nil {
		return err
	}
	return nil
}

// A NodeNotFoundError reports that a specified node could not be found.
type NodeNotFoundError uint32

func (e NodeNotFoundError) Error() string {
	return fmt.Sprintf("node not found: %d", e)
}

// A ConfigNotFoundError reports that a specified configuration could not be
// found.
type ConfigNotFoundError uint32

func (e ConfigNotFoundError) Error() string {
	return fmt.Sprintf("configuration not found: %d", e)
}

// An IllegalConfigError reports that a specified configuration could not be
// created.
type IllegalConfigError string

func (e IllegalConfigError) Error() string {
	return "illegal configuration: " + string(e)
}

// ManagerCreationError returns an error reporting that a Manager could not be
// created due to err.
func ManagerCreationError(err error) error {
	return fmt.Errorf("could not create manager: %s", err.Error())
}

// A QuorumCallError is used to report that a quorum call failed.
type QuorumCallError struct {
	Reason     string
	ReplyCount int
	Errors     []GRPCError
}

func (e QuorumCallError) Error() string {
	var b bytes.Buffer
	b.WriteString("quorum call error: ")
	b.WriteString(e.Reason)
	b.WriteString(fmt.Sprintf(" (errors: %d, replies: %d)", len(e.Errors), e.ReplyCount))
	if len(e.Errors) == 0 {
		return b.String()
	}
	b.WriteString("\ngrpc errors:\n")
	for _, err := range e.Errors {
		b.WriteByte('\t')
		b.WriteString(fmt.Sprintf("node %d: %v", err.NodeID, err.Cause))
		b.WriteByte('\n')
	}
	return b.String()
}

// GRPCError is used to report that a single gRPC call failed.
type GRPCError struct {
	NodeID uint32
	Cause  error
}

func (e GRPCError) Error() string {
	return fmt.Sprintf("node %d: %v", e.NodeID, e.Cause.Error())
}

// LevelNotSet is the zero value level used to indicate that no level (and
// thereby no reply) has been set for a correctable quorum call.
const LevelNotSet = -1

// Manager manages a pool of node configurations on which quorum remote
// procedure calls can be made.
type Manager struct {
	mu       sync.Mutex
	nodes    []*Node
	lookup   map[uint32]*Node
	configs  map[uint32]*Configuration
	eventLog trace.EventLog

	closeOnce sync.Once
	logger    *log.Logger
	opts      managerOptions

	*receiveQueue
}

// NewManager attempts to connect to the given set of node addresses and if
// successful returns a new Manager containing connections to those nodes.
func NewManager(nodeAddrs []string, opts ...ManagerOption) (*Manager, error) {
	if len(nodeAddrs) == 0 {
		return nil, fmt.Errorf("could not create manager: no nodes provided")
	}

	m := &Manager{
		lookup:       make(map[uint32]*Node),
		configs:      make(map[uint32]*Configuration),
		receiveQueue: newReceiveQueue(),
		opts:         newManagerOptions(),
	}

	for _, opt := range opts {
		opt(&m.opts)
	}

	m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithDefaultCallOptions(
		grpc.CallContentSubtype(gorumsContentType),
		grpc.ForceCodec(newGorumsCodec()),
	))

	if m.opts.backoff != backoff.DefaultConfig {
		m.opts.grpcDialOpts = append(m.opts.grpcDialOpts, grpc.WithConnectParams(
			grpc.ConnectParams{Backoff: m.opts.backoff},
		))
	}

	for _, naddr := range nodeAddrs {
		node, err2 := m.createNode(naddr)
		if err2 != nil {
			return nil, ManagerCreationError(err2)
		}
		m.lookup[node.id] = node
		m.nodes = append(m.nodes, node)
	}

	if m.opts.trace {
		title := strings.Join(nodeAddrs, ",")
		m.eventLog = trace.NewEventLog("gorums.Manager", title)
	}

	err := m.connectAll()
	if err != nil {
		return nil, ManagerCreationError(err)
	}

	if m.opts.logger != nil {
		m.logger = m.opts.logger
	}

	if m.eventLog != nil {
		m.eventLog.Printf("ready")
	}

	return m, nil
}

func (m *Manager) createNode(addr string) (*Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("create node %s error: %v", addr, err)
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(tcpAddr.String()))
	id := h.Sum32()

	if _, found := m.lookup[id]; found {
		return nil, fmt.Errorf("create node %s error: node already exists", addr)
	}

	node := &Node{
		id:      id,
		addr:    tcpAddr.String(),
		latency: -1 * time.Second,
	}
	node.createOrderedStream(m.receiveQueue, m.opts)

	return node, nil
}

func (m *Manager) connectAll() error {
	if m.opts.noConnect {
		return nil
	}

	if m.eventLog != nil {
		m.eventLog.Printf("connecting")
	}

	for _, node := range m.nodes {
		err := node.connect(m.opts)
		if err != nil {
			if m.eventLog != nil {
				m.eventLog.Errorf("connect failed, error connecting to node %s, error: %v", node.addr, err)
			}
			return fmt.Errorf("connect node %s error: %v", node.addr, err)
		}
	}
	return nil
}

func (m *Manager) closeNodeConns() {
	for _, node := range m.nodes {
		err := node.close()
		if err == nil {
			continue
		}
		if m.logger != nil {
			m.logger.Printf("node %d: error closing: %v", node.id, err)
		}
	}
}

// Close closes all node connections and any client streams.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		if m.eventLog != nil {
			m.eventLog.Printf("closing")
		}
		m.closeNodeConns()
	})
}

// NodeIDs returns the identifier of each available node. IDs are returned in
// the same order as they were provided in the creation of the Manager.
func (m *Manager) NodeIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.nodes))
	for _, node := range m.nodes {
		ids = append(ids, node.ID())
	}
	return ids
}

// Node returns the node with the given identifier if present.
func (m *Manager) Node(id uint32) (node *Node, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	node, found = m.lookup[id]
	return node, found
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
func (m *Manager) Nodes() []*Node {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nodes
}

// ConfigurationIDs returns the identifier of each available
// configuration.
func (m *Manager) ConfigurationIDs() []uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]uint32, 0, len(m.configs))
	for id := range m.configs {
		ids = append(ids, id)
	}
	return ids
}

// Configuration returns the configuration with the given global
// identifier if present.
func (m *Manager) Configuration(id uint32) (config *Configuration, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	config, found = m.configs[id]
	return config, found
}

// Configurations returns a slice of each available configuration.
func (m *Manager) Configurations() []*Configuration {
	m.mu.Lock()
	defer m.mu.Unlock()
	configs := make([]*Configuration, 0, len(m.configs))
	for _, conf := range m.configs {
		configs = append(configs, conf)
	}
	return configs
}

// Size returns the number of nodes and configurations in the Manager.
func (m *Manager) Size() (nodes, configs int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.nodes), len(m.configs)
}

// AddNode attempts to dial to the provide node address. The node is
// added to the Manager's pool of nodes if a connection was established.
func (m *Manager) AddNode(addr string) error {
	panic("not implemented")
}

// NewConfiguration returns a new configuration given quorum specification and
// a timeout.
func (m *Manager) NewConfiguration(ids []uint32, qspec QuorumSpec) (*Configuration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(ids) == 0 {
		return nil, IllegalConfigError("need at least one node")
	}

	var nodes []*Node
	unique := make(map[uint32]struct{})
	var uniqueIDs []uint32
	for _, nid := range ids {
		// ensure that identical IDs are only counted once
		if _, duplicate := unique[nid]; duplicate {
			continue
		}
		unique[nid] = struct{}{}
		uniqueIDs = append(uniqueIDs, nid)

		node, found := m.lookup[nid]
		if !found {
			return nil, NodeNotFoundError(nid)
		}
		nodes = append(nodes, node)
	}

	// node IDs are sorted to ensure a globally consistent configuration ID
	sort.Sort(idSlice(uniqueIDs))

	h := fnv.New32a()
	for _, id := range uniqueIDs {
		binary.Write(h, binary.LittleEndian, id)
	}
	cid := h.Sum32()

	conf, found := m.configs[cid]
	if found {
		return conf, nil
	}

	c := &Configuration{
		id:    cid,
		nodes: nodes,
		n:     len(nodes),
		mgr:   m,
		qspec: qspec,
	}
	m.configs[cid] = c

	return c, nil
}

type idSlice []uint32

func (p idSlice) Len() int { return len(p) }

func (p idSlice) Less(i, j int) bool { return p[i] < p[j] }

func (p idSlice) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

const nilAngleString = "<nil>"

// Node encapsulates the state of a node on which a remote procedure call
// can be performed.
type Node struct {
	// Only assigned at creation.
	id      uint32
	addr    string
	conn    *grpc.ClientConn
	cancel  func()
	mu      sync.Mutex
	lastErr error
	latency time.Duration

	*orderedNodeStream

	// embed generated nodeServices
	nodeServices
}

func (n *Node) createOrderedStream(rq *receiveQueue, opts managerOptions) {
	n.orderedNodeStream = &orderedNodeStream{
		receiveQueue: rq,
		sendQ:        make(chan *gorumsMessage, opts.sendBuffer),
		node:         n,
		backoff:      opts.backoff,
		rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// connect to this node to facilitate gRPC calls and optionally client streams.
func (n *Node) connect(opts managerOptions) error {
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), opts.nodeDialTimeout)
	defer cancel()
	n.conn, err = grpc.DialContext(ctx, n.addr, opts.grpcDialOpts...)
	if err != nil {
		return fmt.Errorf("dialing node failed: %w", err)
	}
	// a context for all of the streams
	ctx, n.cancel = context.WithCancel(context.Background())
	// only start ordering RPCs when needed
	if hasOrderingMethods {
		err = n.connectOrderedStream(ctx, n.conn)
		if err != nil {
			return fmt.Errorf("starting stream failed: %w", err)
		}
	}
	return n.connectStream(ctx) // call generated method
}

// close this node for further calls and optionally stream.
func (n *Node) close() error {
	if err := n.conn.Close(); err != nil {
		return fmt.Errorf("%d: conn close error: %w", n.id, err)
	}
	err := n.closeStream() // call generated method
	n.cancel()
	return err
}

// ID returns the ID of n.
func (n *Node) ID() uint32 {
	if n != nil {
		return n.id
	}
	return 0
}

// Address returns network address of n.
func (n *Node) Address() string {
	if n != nil {
		return n.addr
	}
	return nilAngleString
}

// Port returns network port of n.
func (n *Node) Port() string {
	if n != nil {
		_, port, _ := net.SplitHostPort(n.addr)
		return port
	}
	return nilAngleString
}

func (n *Node) String() string {
	if n != nil {
		return fmt.Sprintf("addr: %s", n.addr)
	}
	return nilAngleString
}

// FullString returns a more descriptive string representation of n that
// includes id, network address and latency information.
func (n *Node) FullString() string {
	if n != nil {
		n.mu.Lock()
		defer n.mu.Unlock()
		return fmt.Sprintf(
			"node %d | addr: %s | latency: %v",
			n.id, n.addr, n.latency,
		)
	}
	return nilAngleString
}

func (n *Node) setLastErr(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lastErr = err
}

// LastErr returns the last error encountered (if any) when invoking a remote
// procedure call on this node.
func (n *Node) LastErr() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.lastErr
}

func (n *Node) setLatency(lat time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.latency = lat
}

// Latency returns the latency of the last successful remote procedure call
// made to this node.
func (n *Node) Latency() time.Duration {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.latency
}

type lessFunc func(n1, n2 *Node) bool

// MultiSorter implements the Sort interface, sorting the nodes within.
type MultiSorter struct {
	nodes []*Node
	less  []lessFunc
}

// Sort sorts the argument slice according to the less functions passed to
// OrderedBy.
func (ms *MultiSorter) Sort(nodes []*Node) {
	ms.nodes = nodes
	sort.Sort(ms)
}

// OrderedBy returns a Sorter that sorts using the less functions, in order.
// Call its Sort method to sort the data.
func OrderedBy(less ...lessFunc) *MultiSorter {
	return &MultiSorter{
		less: less,
	}
}

// Len is part of sort.Interface.
func (ms *MultiSorter) Len() int {
	return len(ms.nodes)
}

// Swap is part of sort.Interface.
func (ms *MultiSorter) Swap(i, j int) {
	ms.nodes[i], ms.nodes[j] = ms.nodes[j], ms.nodes[i]
}

// Less is part of sort.Interface. It is implemented by looping along the
// less functions until it finds a comparison that is either Less or
// !Less. Note that it can call the less functions twice per call. We
// could change the functions to return -1, 0, 1 and reduce the
// number of calls for greater efficiency: an exercise for the reader.
func (ms *MultiSorter) Less(i, j int) bool {
	p, q := ms.nodes[i], ms.nodes[j]
	// Try all but the last comparison.
	var k int
	for k = 0; k < len(ms.less)-1; k++ {
		less := ms.less[k]
		switch {
		case less(p, q):
			// p < q, so we have a decision.
			return true
		case less(q, p):
			// p > q, so we have a decision.
			return false
		}
		// p == q; try the next comparison.
	}
	// All comparisons to here said "equal", so just return whatever
	// the final comparison reports.
	return ms.less[k](p, q)
}

// ID sorts nodes by their identifier in increasing order.
var ID = func(n1, n2 *Node) bool {
	return n1.id < n2.id
}

// Port sorts nodes by their port number in increasing order.
// Warning: This function may be removed in the future.
var Port = func(n1, n2 *Node) bool {
	p1, _ := strconv.Atoi(n1.Port())
	p2, _ := strconv.Atoi(n2.Port())
	return p1 < p2
}

// Latency sorts nodes by latency in increasing order. Latencies less then
// zero (sentinel value) are considered greater than any positive latency.
var Latency = func(n1, n2 *Node) bool {
	if n1.latency < 0 {
		return false
	}
	return n1.latency < n2.latency
}

// Error sorts nodes by their LastErr() status in increasing order. A
// node with LastErr() != nil is larger than a node with LastErr() == nil.
var Error = func(n1, n2 *Node) bool {
	if n1.lastErr != nil && n2.lastErr == nil {
		return false
	}
	return true
}

type managerOptions struct {
	grpcDialOpts    []grpc.DialOption
	nodeDialTimeout time.Duration
	logger          *log.Logger
	noConnect       bool
	trace           bool
	backoff         backoff.Config
	sendBuffer      uint
}

func newManagerOptions() managerOptions {
	return managerOptions{
		backoff:    backoff.DefaultConfig,
		sendBuffer: 0,
	}
}

// ManagerOption provides a way to set different options on a new Manager.
type ManagerOption func(*managerOptions)

// WithDialTimeout returns a ManagerOption which is used to set the dial
// context timeout to be used when initially connecting to each node in its pool.
func WithDialTimeout(timeout time.Duration) ManagerOption {
	return func(o *managerOptions) {
		o.nodeDialTimeout = timeout
	}
}

// WithGrpcDialOptions returns a ManagerOption which sets any gRPC dial options
// the Manager should use when initially connecting to each node in its pool.
func WithGrpcDialOptions(opts ...grpc.DialOption) ManagerOption {
	return func(o *managerOptions) {
		o.grpcDialOpts = append(o.grpcDialOpts, opts...)
	}
}

// WithLogger returns a ManagerOption which sets an optional error logger for
// the Manager.
func WithLogger(logger *log.Logger) ManagerOption {
	return func(o *managerOptions) {
		o.logger = logger
	}
}

// WithNoConnect returns a ManagerOption which instructs the Manager not to
// connect to any of its nodes. Mainly used for testing purposes.
func WithNoConnect() ManagerOption {
	return func(o *managerOptions) {
		o.noConnect = true
	}
}

// WithTracing controls whether to trace quorum calls for this Manager instance
// using the golang.org/x/net/trace package. Tracing is currently only supported
// for regular quorum calls.
func WithTracing() ManagerOption {
	return func(o *managerOptions) {
		o.trace = true
	}
}

// WithBackoff allows for changing the backoff delays used by Gorums.
func WithBackoff(backoff backoff.Config) ManagerOption {
	return func(o *managerOptions) {
		o.backoff = backoff
	}
}

// WithSendBufferSize allows for changing the size of the send buffer used by Gorums.
// A larger buffer might achieve higher throughput for asynchronous calltypes, but at
// the cost of latency.
func WithSendBufferSize(size uint) ManagerOption {
	return func(o *managerOptions) {
		o.sendBuffer = size
	}
}

type methodInfo struct {
	oneway       bool
	concurrent   bool
	requestType  protoreflect.Message
	responseType protoreflect.Message
}

type orderingResult struct {
	nid   uint32
	reply protoreflect.ProtoMessage
	err   error
}

type receiveQueue struct {
	msgID    uint64
	recvQ    map[uint64]chan *orderingResult
	recvQMut sync.RWMutex
}

func newReceiveQueue() *receiveQueue {
	return &receiveQueue{
		recvQ: make(map[uint64]chan *orderingResult),
	}
}

func (m *receiveQueue) nextMsgID() uint64 {
	return atomic.AddUint64(&m.msgID, 1)
}

func (m *receiveQueue) putChan(id uint64, c chan *orderingResult) {
	m.recvQMut.Lock()
	m.recvQ[id] = c
	m.recvQMut.Unlock()
}

func (m *receiveQueue) deleteChan(id uint64) {
	m.recvQMut.Lock()
	delete(m.recvQ, id)
	m.recvQMut.Unlock()
}

func (m *receiveQueue) putResult(id uint64, result *orderingResult) {
	m.recvQMut.RLock()
	c, ok := m.recvQ[id]
	m.recvQMut.RUnlock()
	if ok {
		c <- result
	}
}

type orderedNodeStream struct {
	*receiveQueue
	sendQ        chan *gorumsMessage
	node         *Node // needed for ID and setLastError
	backoff      backoff.Config
	rand         *rand.Rand
	gorumsClient ordering.GorumsClient
	gorumsStream ordering.Gorums_NodeStreamClient
	streamMut    sync.RWMutex
	streamBroken bool
}

func (s *orderedNodeStream) connectOrderedStream(ctx context.Context, conn *grpc.ClientConn) error {
	var err error
	s.gorumsClient = ordering.NewGorumsClient(conn)
	s.gorumsStream, err = s.gorumsClient.NodeStream(ctx)
	if err != nil {
		return err
	}
	go s.sendMsgs(ctx)
	go s.recvMsgs(ctx)
	return nil
}

func (s *orderedNodeStream) sendMsgs(ctx context.Context) {
	var req *gorumsMessage
	for {
		select {
		case <-ctx.Done():
			return
		case req = <-s.sendQ:
		}
		// return error if stream is broken
		if s.streamBroken {
			err := status.Errorf(codes.Unavailable, "stream is down")
			s.putResult(req.metadata.MessageID, &orderingResult{nid: s.node.ID(), reply: nil, err: err})
			continue
		}
		// else try to send message
		s.streamMut.RLock()
		err := s.gorumsStream.SendMsg(req)
		if err == nil {
			s.streamMut.RUnlock()
			continue
		}
		s.streamBroken = true
		s.streamMut.RUnlock()
		s.node.setLastErr(err)
		// return the error
		s.putResult(req.metadata.MessageID, &orderingResult{nid: s.node.ID(), reply: nil, err: err})
	}
}

func (s *orderedNodeStream) recvMsgs(ctx context.Context) {
	for {
		resp := newGorumsMessage(true)
		s.streamMut.RLock()
		err := s.gorumsStream.RecvMsg(resp)
		if err != nil {
			s.streamBroken = true
			s.streamMut.RUnlock()
			s.node.setLastErr(err)
			// attempt to reconnect
			s.reconnectStream(ctx)
		} else {
			s.streamMut.RUnlock()
			s.putResult(resp.metadata.MessageID, &orderingResult{nid: s.node.ID(), reply: resp.message, err: nil})
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (s *orderedNodeStream) reconnectStream(ctx context.Context) {
	s.streamMut.Lock()
	defer s.streamMut.Unlock()

	var retries float64
	for {
		var err error
		s.gorumsStream, err = s.gorumsClient.NodeStream(ctx)
		if err == nil {
			s.streamBroken = false
			return
		}
		s.node.setLastErr(err)
		delay := float64(s.backoff.BaseDelay)
		max := float64(s.backoff.MaxDelay)
		for r := retries; delay < max && r > 0; r-- {
			delay *= s.backoff.Multiplier
		}
		delay = math.Min(delay, max)
		delay *= 1 + s.backoff.Jitter*(rand.Float64()*2-1)
		select {
		case <-time.After(time.Duration(delay)):
			retries++
		case <-ctx.Done():
			return
		}
	}
}

// requestHandler is used to fetch a response message based on the request.
// A requestHandler should receive a message from the server, unmarshal it into
// the proper type for that Method's request type, call a user provided Handler,
// and return a marshaled result to the server.
type requestHandler func(*gorumsMessage) *gorumsMessage

type orderingServer struct {
	handlers map[int32]requestHandler
	opts     *serverOptions
	ordering.UnimplementedGorumsServer
}

func newOrderingServer(opts *serverOptions) *orderingServer {
	s := &orderingServer{
		handlers: make(map[int32]requestHandler),
		opts:     opts,
	}
	return s
}

func (s *orderingServer) NodeStream(srv ordering.Gorums_NodeStreamServer) error {
	finished := make(chan *gorumsMessage, s.opts.buffer)
	ordered := make(chan struct {
		msg  *gorumsMessage
		info methodInfo
	}, s.opts.buffer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handleMsg := func(req *gorumsMessage, info methodInfo) {
		if handler, ok := s.handlers[req.metadata.MethodID]; ok {
			if info.oneway {
				handler(req)
			} else {
				finished <- handler(req)
			}
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-finished:
				err := srv.SendMsg(msg)
				if err != nil {
					return
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-ordered:
				handleMsg(req.msg, req.info)
			}
		}
	}()

	for {
		req := newGorumsMessage(false)
		err := srv.RecvMsg(req)
		if err != nil {
			return err
		}

		if info, ok := orderingMethods[req.metadata.MethodID]; ok {
			if info.concurrent {
				go handleMsg(req, info)
			} else {
				ordered <- struct {
					msg  *gorumsMessage
					info methodInfo
				}{req, info}
			}
		}
	}
}

type serverOptions struct {
	buffer   uint
	grpcOpts []grpc.ServerOption
}

// ServerOption is used to change settings for the GorumsServer
type ServerOption func(*serverOptions)

// WithServerBufferSize sets the buffer size for the server.
// A larger buffer may result in higher throughput at the cost of higher latency.
func WithServerBufferSize(size uint) ServerOption {
	return func(o *serverOptions) {
		o.buffer = size
	}
}

func WithGRPCServerOptions(opts ...grpc.ServerOption) ServerOption {
	return func(o *serverOptions) {
		o.grpcOpts = append(o.grpcOpts, opts...)
	}
}

// GorumsServer serves all ordering based RPCs using registered handlers.
type GorumsServer struct {
	srv        *orderingServer
	grpcServer *grpc.Server
	opts       serverOptions
}

// NewGorumsServer returns a new instance of GorumsServer.
func NewGorumsServer(opts ...ServerOption) *GorumsServer {
	var serverOpts serverOptions
	for _, opt := range opts {
		opt(&serverOpts)
	}
	serverOpts.grpcOpts = append(serverOpts.grpcOpts, grpc.CustomCodec(newGorumsCodec()))
	s := &GorumsServer{
		srv:        newOrderingServer(&serverOpts),
		grpcServer: grpc.NewServer(serverOpts.grpcOpts...),
	}
	ordering.RegisterGorumsServer(s.grpcServer, s.srv)
	return s
}

// Serve starts serving on the listener.
func (s *GorumsServer) Serve(listener net.Listener) error {
	return s.grpcServer.Serve(listener)
}

// GracefulStop waits for all RPCs to finish before stopping.
func (s *GorumsServer) GracefulStop() {
	s.grpcServer.GracefulStop()
}

// Stop stops the server immediately.
func (s *GorumsServer) Stop() {
	s.grpcServer.Stop()
}

type traceInfo struct {
	trace.Trace
	firstLine firstLine
}

type firstLine struct {
	deadline time.Duration
	cid      uint32
}

func (f *firstLine) String() string {
	var line bytes.Buffer
	io.WriteString(&line, "QC: to config")
	fmt.Fprintf(&line, "%v deadline:", f.cid)
	if f.deadline != 0 {
		fmt.Fprint(&line, f.deadline)
	} else {
		io.WriteString(&line, "none")
	}
	return line.String()
}

type payload struct {
	sent bool
	id   uint32
	msg  interface{}
}

func (p payload) String() string {
	if p.sent {
		return fmt.Sprintf("sent: %v", p.msg)
	}
	return fmt.Sprintf("recv from %d: %v", p.id, p.msg)
}

type qcresult struct {
	ids   []uint32
	reply interface{}
	err   error
}

func (q qcresult) String() string {
	var out bytes.Buffer
	io.WriteString(&out, "recv QC reply: ")
	fmt.Fprintf(&out, "ids: %v, ", q.ids)
	fmt.Fprintf(&out, "reply: %v ", q.reply)
	if q.err != nil {
		fmt.Fprintf(&out, ", error: %v", q.err)
	}
	return out.String()
}

func appendIfNotPresent(set []uint32, x uint32) []uint32 {
	for _, y := range set {
		if y == x {
			return set
		}
	}
	return append(set, x)
}

`
