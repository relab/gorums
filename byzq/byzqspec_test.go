package byzq

import (
	"strings"
	"testing"
)

var byzQTests = []struct {
	n   int
	f   int // expected value
	q   int // expected value
	err string
}{
	{4, 0, 2, "Byzantine masking quorums require n>4f replicas; only got n=4, yielding f=0"},
	{5, 1, 3, ""},
	{6, 1, 4, ""},
	{7, 1, 4, ""},
	{8, 1, 5, ""},
	{9, 2, 6, ""},
	{10, 2, 7, ""},
	{11, 2, 7, ""},
	{12, 2, 8, ""},
	{13, 3, 9, ""},
	{14, 3, 10, ""},
}

func TestByzQ(t *testing.T) {
	for _, test := range byzQTests {
		bq, err := NewByzQ(test.n)
		if err != nil {
			if err.Error() != test.err {
				t.Errorf("got '%v', expected '%v'", err.Error(), test.err)
			}
			continue
		}
		if bq.f != test.f {
			t.Errorf("got f=%d, expected f=%d", bq.f, test.f)
		}
		if bq.q != test.q {
			t.Errorf("got q=%d, expected q=%d", bq.q, test.q)
		}
	}

}

var byzReadQFTests = []struct {
	name     string
	replies  []*Value
	expected *Value
	rq       bool
}{
	{
		"nil input",
		nil,
		nil,
		false,
	},
	{
		"len=0 input",
		[]*Value{},
		nil,
		false,
	},
	{
		"no quorum (I)",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 1}},
		},
		nil,
		false,
	},
	{
		"no quorum (II)",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 1}},
		},
		nil,
		false,
	},
	{
		"no quorum (III); default value",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "3", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "4", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 1}},
		},
		&defaultVal,
		true,
	},
	{
		"no quorum (IV); default value",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "3", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 2}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 3}},
			{C: &Content{Key: "winnie", Value: "4", Timestamp: 1}},
		},
		&defaultVal,
		true,
	},
	{
		//todo: decide if #replies > n should be accepted ?
		"no quorum (V); default value",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "3", Timestamp: 2}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 3}},
			{C: &Content{Key: "winnie", Value: myVal.C.Value, Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 2}},
			{C: &Content{Key: "winnie", Value: "4", Timestamp: 3}},
		},
		&defaultVal,
		true,
	},
	{
		"quorum (I)",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myVal,
		true,
	},
	{
		"quorum (II)",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			myVal,
			myVal,
			myVal,
		},
		myVal,
		true,
	},
	{
		"quorum (III)",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myVal,
		true,
	},
	{
		"quorum (IV)",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			myVal,
			myVal,
			myVal,
		},
		myVal,
		true,
	},
	{
		"base-case quorum",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myVal,
		true,
	},
	{
		"approx. worst-case quorum",
		[]*Value{
			{C: &Content{Key: "winnie", Value: "2", Timestamp: 1}},
			{C: &Content{Key: "winnie", Value: "4", Timestamp: 2}},
			{C: &Content{Key: "winnie", Value: "5", Timestamp: 1}},
			myVal,
			myVal,
		},
		myVal,
		true,
	},
}

func TestByzReadQF(t *testing.T) {
	qspec, err := NewByzQ(5)
	if err != nil {
		t.Error(err)
	}
	for _, test := range byzReadQFTests {
		t.Run("ByzQ(5,1)-"+test.name, func(t *testing.T) {
			reply, byzquorum := qspec.ReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if !reply.Equal(test.expected) {
				t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
			}
		})
	}
}

func BenchmarkByzReadQF(b *testing.B) {
	qspec, err := NewByzQ(5)
	if err != nil {
		b.Error(err)
	}
	for _, test := range byzReadQFTests {
		if !strings.Contains(test.name, "case") {
			continue
		}
		b.Run("ByzQ(5,1)-"+test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.ReadQF(test.replies)
			}
		})
	}
}
