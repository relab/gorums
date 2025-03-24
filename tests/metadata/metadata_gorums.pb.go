// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.8.0-devel
// 	protoc            v5.29.2
// source: metadata/metadata.proto

package metadata

import (
	context "context"
	gorums "github.com/relab/gorums"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(8 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 8)
)

// MetadataTestNodeClient is the single node client interface for the MetadataTest service.
type MetadataTestNodeClient interface {
	IDFromMD(ctx context.Context, in *emptypb.Empty) (resp *NodeID, err error)
	WhatIP(ctx context.Context, in *emptypb.Empty) (resp *IPAddr, err error)
}

// enforce interface compliance
var _ MetadataTestNodeClient = (*MetadataTestNode)(nil)

// A MetadataTestConfiguration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type MetadataTestConfiguration struct {
	gorums.RawConfiguration
	nodes []*MetadataTestNode
} // MetadataTestQuorumSpecFromRaw returns a new MetadataTestQuorumSpec from the given raw configuration.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func MetadataTestConfigurationFromRaw(rawCfg gorums.RawConfiguration) (*MetadataTestConfiguration, error) {
	newCfg := &MetadataTestConfiguration{
		RawConfiguration: rawCfg,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*MetadataTestNode, newCfg.Size())
	for i, n := range rawCfg {
		newCfg.nodes[i] = &MetadataTestNode{n}
	}
	return newCfg, nil
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *MetadataTestConfiguration) Nodes() []*MetadataTestNode {
	return c.nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c MetadataTestConfiguration) And(d *MetadataTestConfiguration) gorums.NodeListOption {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c MetadataTestConfiguration) Except(rm *MetadataTestConfiguration) gorums.NodeListOption {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}

// MetadataTestManager maintains a connection pool of nodes on
// which quorum calls can be performed.
type MetadataTestManager struct {
	*gorums.RawManager
}

// NewMetadataTestManager returns a new MetadataTestManager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewMetadataTestManager(opts ...gorums.ManagerOption) *MetadataTestManager {
	return &MetadataTestManager{
		RawManager: gorums.NewRawManager(opts...),
	}
} // NewMetadataTestConfiguration returns a MetadataTestConfiguration based on the provided list of nodes (required)
// .
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *MetadataTestManager) NewConfiguration(cfg gorums.NodeListOption) (c *MetadataTestConfiguration, err error) {
	c = &MetadataTestConfiguration{}
	c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, cfg)
	if err != nil {
		return nil, err
	}
	// initialize the nodes slice
	c.nodes = make([]*MetadataTestNode, c.Size())
	for i, n := range c.RawConfiguration {
		c.nodes[i] = &MetadataTestNode{n}
	}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *MetadataTestManager) Nodes() []*MetadataTestNode {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*MetadataTestNode, len(gorumsNodes))
	for i, n := range gorumsNodes {
		nodes[i] = &MetadataTestNode{n}
	}
	return nodes
}

// MetadataTestNode holds the node specific methods for the MetadataTest service.
type MetadataTestNode struct {
	*gorums.RawNode
}

// IDFromMD returns the 'id' field from the metadata.
func (n *MetadataTestNode) IDFromMD(ctx context.Context, in *emptypb.Empty) (resp *NodeID, err error) {
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
func (n *MetadataTestNode) WhatIP(ctx context.Context, in *emptypb.Empty) (resp *IPAddr, err error) {
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
