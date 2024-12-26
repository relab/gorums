package logging

import (
	"log/slog"
	"time"
)

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
	MaxRetries      int       `json:"maxRetries"`
	NumFailed       int       `json:"numFailed"`
	Stopping        bool      `json:"stopping"`
	IsBroadcastCall bool      `json:"isBroadcastCall"`
	Started         time.Time `json:"started"`
	Ended           time.Time `json:"ended"`
}

// funcs: used to get type safety on fields when logging
func MsgType(msgType string) slog.Attr {
	return slog.String("msgType", msgType)
}

func BroadcastID(broadcastID uint64) slog.Attr {
	return slog.Uint64("BroadcastID", broadcastID)
}

func Err(err error) slog.Attr {
	return slog.Any("err", err)
}

func Method(m string) slog.Attr {
	return slog.String("method", m)
}

func From(from string) slog.Attr {
	return slog.String("from", from)
}

func Cancelled(cancelled bool) slog.Attr {
	return slog.Bool("cancelled", cancelled)
}

func MachineID(machineID uint64) slog.Attr {
	return slog.Uint64("MachineID", machineID)
}

func MsgID(msgID uint64) slog.Attr {
	return slog.Uint64("msgID", msgID)
}

func NodeID(nodeID uint32) slog.Attr {
	return slog.Uint64("nodeID", uint64(nodeID))
}

func NodeAddr(nodeAddr string) slog.Attr {
	return slog.String("nodeAddr", nodeAddr)
}

func Type(t string) slog.Attr {
	return slog.String("type", t)
}

func Reconnect(reconnect bool) slog.Attr {
	return slog.Bool("reconnect", reconnect)
}

func RetryNum(num float64) slog.Attr {
	return slog.Float64("retryNum", num)
}

func MaxRetries(maxRetries int) slog.Attr {
	return slog.Int("maxRetries", maxRetries)
}

func NumFailed(num int) slog.Attr {
	return slog.Int("numFailed", num)
}

func Stopping(stopping bool) slog.Attr {
	return slog.Bool("stopping", stopping)
}

func IsBroadcastCall(isBroadcastCall bool) slog.Attr {
	return slog.Bool("isBroadcastCall", isBroadcastCall)
}

func Started(started time.Time) slog.Attr {
	return slog.Time("started", started)
}

func Ended(ended time.Time) slog.Attr {
	return slog.Time("ended", ended)
}
