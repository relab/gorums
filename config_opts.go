package gorums

type configOptions struct {
	idMapping map[string]uint32
	addrsList []string
	nodeIDs   []uint32
}

func newConfigOptions() configOptions {
	return configOptions{}
}

// ConfigOption provides a way to set different options on a new Manager.
type ConfigOption func(*configOptions)

// WithNodeMap returns a ConfigOption containing the provided mapping from node addresses to application-specific IDs.
func WithNodeMap(idMap map[string]uint32) ConfigOption {
	return func(o *configOptions) {
		o.idMapping = idMap
	}
}

// WithNodeList returns a ConfigOption containing the provided list of node addresses.
// With this option, node IDs are generated by the Manager.
func WithNodeList(addrsList []string) ConfigOption {
	return func(o *configOptions) {
		o.addrsList = addrsList
	}
}

// WithNodeIDs returns a ConfigOption containing a list of node IDs.
// This assumes that the provided node IDs have already been registered with the manager.
func WithNodeIDs(nodeIDs []uint32) ConfigOption {
	return func(o *configOptions) {
		o.nodeIDs = nodeIDs
	}
}
