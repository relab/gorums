// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.9.0-devel+ee38614a
// 	protoc            v5.29.3
// source: metadata/metadata.proto

package metadata

import (
	context "context"
	fmt "fmt"
	gorums "github.com/relab/gorums"
	encoding "google.golang.org/grpc/encoding"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(9 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 9)
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
	var test any = struct{}{}
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
	var test any = struct{}{}
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

// MetadataTestNodeClient is the single node client interface for the MetadataTest service.
type MetadataTestNodeClient interface {
	IDFromMD(ctx context.Context, in *emptypb.Empty) (resp *NodeID, err error)
	WhatIP(ctx context.Context, in *emptypb.Empty) (resp *IPAddr, err error)
}

// enforce interface compliance
var _ MetadataTestNodeClient = (*Node)(nil)

// There are no quorum calls.
type QuorumSpec interface{}

// IDFromMD returns the 'id' field from the metadata.
func (n *Node) IDFromMD(ctx context.Context, in *emptypb.Empty) (resp *NodeID, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "metadata.MetadataTest.IDFromMD",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*NodeID), err
}

// WhatIP returns the address of the client that calls it.
func (n *Node) WhatIP(ctx context.Context, in *emptypb.Empty) (resp *IPAddr, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "metadata.MetadataTest.WhatIP",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*IPAddr), err
}

// MetadataTest is the server-side API for the MetadataTest Service
type MetadataTestServer interface {
	IDFromMD(ctx gorums.ServerCtx, request *emptypb.Empty) (response *NodeID, err error)
	WhatIP(ctx gorums.ServerCtx, request *emptypb.Empty) (response *IPAddr, err error)
}

func RegisterMetadataTestServer(srv *gorums.Server, impl MetadataTestServer) {
	srv.RegisterHandler("metadata.MetadataTest.IDFromMD", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		resp, err := impl.IDFromMD(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
	srv.RegisterHandler("metadata.MetadataTest.WhatIP", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*emptypb.Empty)
		defer ctx.Release()
		resp, err := impl.WhatIP(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
}
