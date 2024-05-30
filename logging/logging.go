package logging

import "time"

// the log entry used in slog with correct types and json mapping
type LogEntry struct {
	Time            time.Time `json:"time"`
	Level           string    `json:"level"`
	Msg             string    `json:"msg"`
	MsgType         string    `json:"msgType"`
	BroadcastID     uint64    `json:"BroadcastID"`
	Err             error     `json:"err"`
	Method          string    `json:"method"`
	From            string    `json:"from"`
	Cancelled       bool      `json:"cancelled"`
	MachineID       uint64    `json:"MachineID"`
	MsgID           uint64    `json:"msgID"`
	NodeID          uint64    `json:"nodeID"`
	NodeAddr        string    `json:"nodeAddr"`
	Type            string    `json:"type"`
	Reconnect       bool      `json:"reconnect"`
	RetryNum        float64   `json:"retryNum"`
	MaxRetries      string    `json:"maxRetries"`
	NumFailed       string    `json:"numFailed"`
	Stopping        bool      `json:"stopping"`
	IsBroadcastCall bool      `json:"isBroadcastCall"`
	Started         time.Time `json:"started"`
	Ended           time.Time `json:"ended"`
}

// enum: used to get type safety on fields when logging
const (
	Msg             string = "msg"
	MsgType         string = "msgType"
	BroadcastID     string = "BroadcastID"
	Err             string = "err"
	Method          string = "method"
	From            string = "from"
	Cancelled       string = "cancelled"
	MachineID       string = "MachineID"
	MsgID           string = "msgID"
	NodeID          string = "nodeID"
	NodeAddr        string = "nodeAddr"
	Type            string = "type"
	Reconnect       string = "reconnect"
	RetryNum        string = "retryNum"
	MaxRetries      string = "maxRetries"
	NumFailed       string = "numFailed"
	Stopping        string = "stopping"
	IsBroadcastCall string = "isBroadcastCall"
	Started         string = "started"
	Ended           string = "ended"
)
