package dev

// ReadQuorumFn is used to pick a reply from the replies if there is a quorum.
// If there was not enough replies to satisfy the quorum requirement,
// then the function returns (nil, false). Otherwise, the function picks a
// reply among the replies and returns (reply, true).
type ReadQuorumFn func(c *Configuration, replies []*State) (*State, bool)

// WriteQuorumFn is used to pick a reply from the replies if there is a quorum.
// If there was not enough replies to satisfy the quorum requirement,
// then the function returns (nil, false). Otherwise, the function picks a
// reply among the replies and returns (reply, true).
type WriteQuorumFn func(c *Configuration, replies []*WriteResponse) (*WriteResponse, bool)
