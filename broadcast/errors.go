package broadcast

type BroadcastIDErr struct{}

func (err BroadcastIDErr) Error() string {
	return "wrong broadcastID"
}

type MissingClientReqErr struct{}

func (err MissingClientReqErr) Error() string {
	return "has not received client req yet"
}

type AlreadyProcessedErr struct{}

func (err AlreadyProcessedErr) Error() string {
	return "already processed request"
}

type OutOfOrderErr struct{}

func (err OutOfOrderErr) Error() string {
	return "the message is out of order"
}

type ShardDownErr struct{}

func (err ShardDownErr) Error() string {
	return "the shard is down"
}

type InvalidAddrErr struct {
	addr string
}

func (err InvalidAddrErr) Error() string {
	return "provided addr is invalid. got: " + err.addr
}
