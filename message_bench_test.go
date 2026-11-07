package gorums

import (
	"testing"

	"github.com/relab/gorums/internal/stream"
)

var benchmarkResponse *stream.Message

// BenchmarkResponseEnvelopeConstruction isolates construction from marshaling.
func BenchmarkResponseEnvelopeConstruction(b *testing.B) {
	in := stream.Message_builder{MessageSeqNo: 7, Method: "test.Method"}.Build()
	for _, tc := range []struct {
		name  string
		build func() *stream.Message
	}{
		{name: "Production", build: func() *stream.Message {
			return newResponse(in, nil, nil)
		}},
		{name: "Builder", build: func() *stream.Message {
			return stream.Message_builder{MessageSeqNo: in.GetMessageSeqNo(), Method: in.GetMethod()}.Build()
		}},
		{name: "Setters", build: func() *stream.Message {
			out := &stream.Message{}
			out.SetMessageSeqNo(in.GetMessageSeqNo())
			out.SetMethod(in.GetMethod())
			return out
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkResponse = tc.build()
			}
		})
	}
}
