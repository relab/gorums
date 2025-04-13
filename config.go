package gorums

import (
	"fmt"

	"github.com/relab/gorums/authentication"
	"github.com/relab/gorums/ordering"
)

// RawConfiguration represents a static set of nodes on which quorum calls may be invoked.
//
// NOTE: mutating the configuration is not supported.
//
// This type is intended to be used by generated code.
// You should use the generated `Configuration` type instead.
type RawConfiguration []*RawNode

// NewRawConfiguration returns a configuration based on the provided list of nodes.
// Nodes can be supplied using WithNodeMap or WithNodeList, or WithNodeIDs.
// A new configuration can also be created from an existing configuration,
// using the And, WithNewNodes, Except, and WithoutNodes methods.
func NewRawConfiguration(mgr *RawManager, opt NodeListOption) (nodes RawConfiguration, err error) {
	if opt == nil {
		return nil, fmt.Errorf("config: missing required node list")
	}
	return opt.newConfig(mgr)
}

// NodeIDs returns a slice of this configuration's Node IDs.
func (c RawConfiguration) NodeIDs() []uint32 {
	ids := make([]uint32, len(c))
	for i, node := range c {
		ids[i] = node.ID()
	}
	return ids
}

// Nodes returns the nodes in this configuration.
//
// NOTE: mutating the returned slice is not supported.
func (c RawConfiguration) Nodes() []*RawNode {
	return c
}

// Size returns the number of nodes in this configuration.
func (c RawConfiguration) Size() int {
	return len(c)
}

// Equal returns true if configurations b and c have the same set of nodes.
func (c RawConfiguration) Equal(b RawConfiguration) bool {
	if len(c) != len(b) {
		return false
	}
	for i := range c {
		if c[i].ID() != b[i].ID() {
			return false
		}
	}
	return true
}

func (c RawConfiguration) getMsgID() uint64 {
	return c[0].mgr.getMsgID()
}

func (c RawConfiguration) sign(msg *Message, signOrigin ...bool) {
	if c[0].mgr.opts.auth != nil {
		if len(signOrigin) > 0 && signOrigin[0] {
			originMsg := authentication.EncodeMsg(msg.Message)
			digest := c[0].mgr.opts.auth.Hash(originMsg)
			originSignature, err := c[0].mgr.opts.auth.Sign(originMsg)
			if err != nil {
				panic(err)
			}
			pubKey, err := c[0].mgr.opts.auth.EncodePublic()
			if err != nil {
				panic(err)
			}
			msg.Metadata.GetBroadcastMsg().SetOriginDigest(digest)
			msg.Metadata.GetBroadcastMsg().SetOriginPubKey(pubKey)
			msg.Metadata.GetBroadcastMsg().SetOriginSignature(originSignature)
		}
		encodedMsg := c.encodeMsg(msg)
		signature, err := c[0].mgr.opts.auth.Sign(encodedMsg)
		if err != nil {
			panic(err)
		}
		msg.Metadata.GetAuthMsg().SetSignature(signature)
	}
}

func (c RawConfiguration) encodeMsg(msg *Message) []byte {
	// we do not want to include the signature field in the signature
	auth := c[0].mgr.opts.auth
	pubKey, err := auth.EncodePublic()
	if err != nil {
		panic(err)
	}
	msg.Metadata.SetAuthMsg(ordering.AuthMsg_builder{
		PublicKey: pubKey,
		Signature: nil,
		Sender:    auth.Addr(),
	}.Build())
	return authentication.EncodeMsg(*msg)
}
