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
