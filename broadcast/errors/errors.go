package errors

// IDErr should be used when a message with a BroadcastID is sent to a broadcast processor with another BroadcastID. This
// can happen if a user deliberately changes the BroadcastID of a message.
type IDErr struct{}

func (err IDErr) Error() string {
	return "broadcast: wrong ID"
}

// MissingClientReqErr signifies that a server tries to reply to a client, but has not yet received the original request
// form the client. This is especially important when the message does not contain routing information, such as in QuorumCalls.
type MissingClientReqErr struct{}

func (err MissingClientReqErr) Error() string {
	return "broadcast: has not received client req yet"
}

// AlreadyProcessedErr is used when a message is received after the broadcast processor has stopped. This means that the
// server has sent a reply to the client and thus the incoming message needs not be processed.
type AlreadyProcessedErr struct{}

func (err AlreadyProcessedErr) Error() string {
	return "broadcast: already processed request"
}

// ClientReqAlreadyReceivedErr should be used when a duplicate client request is received.
type ClientReqAlreadyReceivedErr struct{}

func (err ClientReqAlreadyReceivedErr) Error() string {
	return "broadcast: client request already received (dropped)"
}

// OutOfOrderErr should be used when the preserve ordering configuration option is used and a message is received out of
// order.
type OutOfOrderErr struct{}

func (err OutOfOrderErr) Error() string {
	return "broadcast: the message is out of order"
}

// InvalidAddrErr should be used when an invalid server/client address is provided.
type InvalidAddrErr struct {
	Addr string
}

func (err InvalidAddrErr) Error() string {
	return "broadcast: provided Addr is invalid. got: " + err.Addr
}
