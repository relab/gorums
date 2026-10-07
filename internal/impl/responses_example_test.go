package impl

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

// exampleResponses returns the Responses of a call to len(replies) nodes
// that received the given replies.
func exampleResponses(replies ...NodeResponse[proto.Message]) *Responses[*pb.StringValue] {
	callCtx, err := newReplayCallContext[*pb.StringValue, *pb.StringValue](context.Background(), len(replies), replies)
	if err != nil {
		panic(err)
	}
	return newResponses(callCtx)
}

// This example shows two custom quorum functions. The first returns the
// response type: the value that a majority of the nodes replied. The second
// returns another type: the sorted values of all successful replies.
func ExampleResponses_customAggregation() {
	majorityValue := func(resp *Responses[*pb.StringValue]) (*pb.StringValue, error) {
		quorum := resp.Size()/2 + 1
		votes := make(map[string]int)
		for r := range resp.Results().IgnoreErrors() {
			votes[r.Value.GetValue()]++
			if votes[r.Value.GetValue()] >= quorum {
				return r.Value, nil
			}
		}
		return nil, ErrIncomplete
	}
	sortedValues := func(resp *Responses[*pb.StringValue]) ([]string, error) {
		replies := resp.Results().IgnoreErrors().CollectAll()
		if len(replies) == 0 {
			return nil, ErrIncomplete
		}
		values := make([]string, 0, len(replies))
		for _, v := range replies {
			values = append(values, v.GetValue())
		}
		slices.Sort(values)
		return values, nil
	}

	replies := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("v1")},
		{NodeID: 2, Value: pb.String("v2")},
		{NodeID: 3, Err: errors.New("node 3 failed")},
		{NodeID: 4, Value: pb.String("v1")},
		{NodeID: 5, Value: pb.String("v1")},
	}
	// A Responses value is single-use, so each quorum function gets its own.
	if v, err := majorityValue(exampleResponses(replies...)); err == nil {
		fmt.Println("majority:", v.GetValue())
	}
	if values, err := sortedValues(exampleResponses(replies...)); err == nil {
		fmt.Println("values:", values)
	}

	// Without a majority for any value, majorityValue reports ErrIncomplete.
	split := []NodeResponse[proto.Message]{
		{NodeID: 1, Value: pb.String("v1")},
		{NodeID: 2, Value: pb.String("v2")},
		{NodeID: 3, Err: errors.New("node 3 failed")},
	}
	if _, err := majorityValue(exampleResponses(split...)); err != nil {
		fmt.Println("majority:", err)
	}

	// Output:
	// majority: v1
	// values: [v1 v1 v1 v2]
	// majority: incomplete call
}
