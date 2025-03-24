// Code generated by protoc-gen-gorums. DO NOT EDIT.
// versions:
// 	protoc-gen-gorums v0.8.0-devel
// 	protoc            v5.29.2
// source: dummy/dummy.proto

package dummy

import (
	context "context"
	gorums "github.com/relab/gorums"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = gorums.EnforceVersion(8 - gorums.MinVersion)
	// Verify that the gorums runtime is sufficiently up-to-date.
	_ = gorums.EnforceVersion(gorums.MaxVersion - 8)
)

// DummyNodeClient is the single node client interface for the Dummy service.
type DummyNodeClient interface {
	Test(ctx context.Context, in *Empty) (resp *Empty, err error)
}

// enforce interface compliance
var _ DummyNodeClient = (*DummyNode)(nil)

// A DummyConfiguration represents a static set of nodes on which quorum remote
// procedure calls may be invoked.
type DummyConfiguration struct {
	gorums.RawConfiguration
	nodes []*DummyNode
} // DummyQuorumSpecFromRaw returns a new DummyQuorumSpec from the given raw configuration.
//
// This function may for example be used to "clone" a configuration but install a different QuorumSpec:
//
//	cfg1, err := mgr.NewConfiguration(qspec1, opts...)
//	cfg2 := ConfigurationFromRaw(cfg1.RawConfig, qspec2)
func DummyConfigurationFromRaw(rawCfg gorums.RawConfiguration) (*DummyConfiguration, error) {
	newCfg := &DummyConfiguration{
		RawConfiguration: rawCfg,
	}
	// initialize the nodes slice
	newCfg.nodes = make([]*DummyNode, newCfg.Size())
	for i, n := range rawCfg {
		newCfg.nodes[i] = &DummyNode{n}
	}
	return newCfg, nil
}

// Nodes returns a slice of each available node. IDs are returned in the same
// order as they were provided in the creation of the Manager.
//
// NOTE: mutating the returned slice is not supported.
func (c *DummyConfiguration) Nodes() []*DummyNode {
	return c.nodes
}

// And returns a NodeListOption that can be used to create a new configuration combining c and d.
func (c DummyConfiguration) And(d *DummyConfiguration) gorums.NodeListOption {
	return c.RawConfiguration.And(d.RawConfiguration)
}

// Except returns a NodeListOption that can be used to create a new configuration
// from c without the nodes in rm.
func (c DummyConfiguration) Except(rm *DummyConfiguration) gorums.NodeListOption {
	return c.RawConfiguration.Except(rm.RawConfiguration)
}

// DummyManager maintains a connection pool of nodes on
// which quorum calls can be performed.
type DummyManager struct {
	*gorums.RawManager
}

// NewDummyManager returns a new DummyManager for managing connection to nodes added
// to the manager. This function accepts manager options used to configure
// various aspects of the manager.
func NewDummyManager(opts ...gorums.ManagerOption) *DummyManager {
	return &DummyManager{
		RawManager: gorums.NewRawManager(opts...),
	}
} // NewDummyConfiguration returns a DummyConfiguration based on the provided list of nodes (required)
// .
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func (m *DummyManager) NewConfiguration(cfg gorums.NodeListOption) (c *DummyConfiguration, err error) {
	c = &DummyConfiguration{}
	c.RawConfiguration, err = gorums.NewRawConfiguration(m.RawManager, cfg)
	if err != nil {
		return nil, err
	}
	// initialize the nodes slice
	c.nodes = make([]*DummyNode, c.Size())
	for i, n := range c.RawConfiguration {
		c.nodes[i] = &DummyNode{n}
	}
	return c, nil
}

// Nodes returns a slice of available nodes on this manager.
// IDs are returned in the order they were added at creation of the manager.
func (m *DummyManager) Nodes() []*DummyNode {
	gorumsNodes := m.RawManager.Nodes()
	nodes := make([]*DummyNode, len(gorumsNodes))
	for i, n := range gorumsNodes {
		nodes[i] = &DummyNode{n}
	}
	return nodes
}

// DummyNode holds the node specific methods for the Dummy service.
type DummyNode struct {
	*gorums.RawNode
}

// Test is a quorum call invoked on all nodes in configuration c,
// with the same argument in, and returns a combined result.
func (n *DummyNode) Test(ctx context.Context, in *Empty) (resp *Empty, err error) {
	cd := gorums.CallData{
		Message: in,
		Method:  "dummy.Dummy.Test",
	}

	res, err := n.RawNode.RPCCall(ctx, cd)
	if err != nil {
		return nil, err
	}
	return res.(*Empty), err
}

// Dummy is the server-side API for the Dummy Service
type DummyServer interface {
	Test(ctx gorums.ServerCtx, request *Empty) (response *Empty, err error)
}

func RegisterDummyServer(srv *gorums.Server, impl DummyServer) {
	srv.RegisterHandler("dummy.Dummy.Test", func(ctx gorums.ServerCtx, in *gorums.Message, finished chan<- *gorums.Message) {
		req := in.Message.(*Empty)
		defer ctx.Release()
		resp, err := impl.Test(ctx, req)
		gorums.SendMessage(ctx, finished, gorums.WrapMessage(in.Metadata, resp, err))
	})
}
