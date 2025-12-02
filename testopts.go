package gorums

// TestOption is a marker interface that can hold ManagerOption,
// ServerOption, or NodeListOption. This allows test helpers to accept
// a single variadic parameter that can be filtered and passed to the
// appropriate constructors (NewManager, NewServer, NewConfiguration).
//
// Each option type (ManagerOption, ServerOption, NodeListOption) embeds
// this interface, so they can be passed directly without wrapping:
//
//	SetupConfiguration(t, 3, nil,
//		WithBackoff(...),           // ManagerOption
//		WithReceiveBufferSize(10),  // ServerOption
//		WithNodeMap(...),           // NodeListOption
//	)
type TestOption any

// testOptions holds extracted options from a slice of TestOption.
type testOptions struct {
	managerOpts  []ManagerOption
	serverOpts   []ServerOption
	nodeListOpts []NodeListOption
}

// extractTestOptions separates a slice of TestOption into their specific types.
func extractTestOptions(opts []TestOption) testOptions {
	var result testOptions
	for _, opt := range opts {
		switch o := opt.(type) {
		case ManagerOption:
			result.managerOpts = append(result.managerOpts, o)
		case ServerOption:
			result.serverOpts = append(result.serverOpts, o)
		case NodeListOption:
			result.nodeListOpts = append(result.nodeListOpts, o)
		}
	}
	return result
}
