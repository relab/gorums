package gridq

import (
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

func TestMain(m *testing.M) {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false
	res := m.Run()
	os.Exit(res)
}

const val = 42

var gridReadQFTests = []struct {
	name    string
	replies []*ReadResponse
	rq      bool
}{
	{
		"nil input",
		nil,
		false,
	},
	{
		"len=0 input",
		[]*ReadResponse{},
		false,
	},
	{
		"no quorum (I)",
		[]*ReadResponse{
			{Row: 0, Col: 0, State: &State{}},
			{Row: 0, Col: 1, State: &State{}},
		},
		false,
	},
	{
		"no quorum (II)",
		[]*ReadResponse{
			{Row: 0, Col: 0, State: &State{}},
			{Row: 1, Col: 0, State: &State{}},
		},
		false,
	},
	{
		"no quorum (III)",
		[]*ReadResponse{
			{Row: 2, Col: 0, State: &State{}},
			{Row: 1, Col: 0, State: &State{}},
			{Row: 0, Col: 0, State: &State{}},
		},
		false,
	},
	{
		"no quorum (IV)",
		[]*ReadResponse{
			{Row: 0, Col: 0, State: &State{}},
			{Row: 1, Col: 1, State: &State{}},
			{Row: 0, Col: 1, State: &State{}},
			{Row: 1, Col: 0, State: &State{}},
		},
		false,
	},
	{
		"no quorum (V)",
		[]*ReadResponse{
			{Row: 2, Col: 2, State: &State{}},
			{Row: 0, Col: 0, State: &State{}},
			{Row: 1, Col: 1, State: &State{}},
			{Row: 0, Col: 1, State: &State{}},
			{Row: 2, Col: 0, State: &State{}},
			{Row: 1, Col: 0, State: &State{}},
		},
		false,
	},
	{
		"col quorum",
		[]*ReadResponse{
			{Row: 0, Col: 1, State: &State{}},
			{Row: 1, Col: 1, State: &State{}},
			{Row: 2, Col: 1, State: &State{}},
		},
		false,
	},
	{
		"best-case quorum",
		[]*ReadResponse{
			{Row: 0, Col: 0, State: &State{Timestamp: 2, Value: 9}},
			{Row: 0, Col: 1, State: &State{Timestamp: 3, Value: val}},
			{Row: 0, Col: 2, State: &State{Timestamp: 1, Value: 3}},
		},
		true,
	},
	{
		"approx. worst-case quorum",
		[]*ReadResponse{
			{Row: 1, Col: 0, State: &State{}},
			{Row: 2, Col: 1, State: &State{Timestamp: 2, Value: 9}},
			{Row: 0, Col: 1, State: &State{}},
			{Row: 1, Col: 1, State: &State{}},
			{Row: 2, Col: 0, State: &State{Timestamp: 3, Value: val}},
			{Row: 0, Col: 0, State: &State{}},
			{Row: 2, Col: 2, State: &State{Timestamp: 1, Value: 3}},
		},
		true,
	},
}

const grows, gcols = 3, 3

var qspecs = []struct {
	name string
	spec QuorumSpec
}{
	{
		"GQSort(3x3)",
		&gqSort{
			rows:      grows,
			cols:      gcols,
			printGrid: false,
			vgrid:     newVisualGrid(grows, gcols),
		},
	},
	{
		"GQMap(3x3)",
		&gqMap{
			rows:      grows,
			cols:      gcols,
			printGrid: false,
			vgrid:     newVisualGrid(grows, gcols),
		},
	},
	{
		"GQSliceOne(3x3)",
		&gqSliceOne{
			rows:      grows,
			cols:      gcols,
			printGrid: false,
			vgrid:     newVisualGrid(grows, gcols),
		},
	},
	{
		"GQSliceTwo(3x3)",
		&gqSliceTwo{
			rows:      grows,
			cols:      gcols,
			printGrid: false,
			vgrid:     newVisualGrid(grows, gcols),
		},
	},
}

func TestGridReadQF(t *testing.T) {
	for _, qspec := range qspecs {
		for _, test := range gridReadQFTests {
			t.Run(qspec.name+"-"+test.name, func(t *testing.T) {
				replies := cloneReplies(test.replies)
				reply, rquorum := qspec.spec.ReadQF(replies)
				if rquorum != test.rq {
					t.Errorf("got %t, want %t", rquorum, test.rq)
				}
				if rquorum {
					if reply == nil || reply.State == nil {
						t.Fatalf("got nil as quorum value, want %d", val)
					}
					gotVal := reply.State.Value
					if gotVal != val {
						t.Errorf("got %d, want %d as quorum value", gotVal, val)
					}
				}
			})
		}
	}
}

func BenchmarkGridReadQF(b *testing.B) {
	for _, qspec := range qspecs {
		for _, test := range gridReadQFTests {
			if !strings.Contains(test.name, "case") {
				continue
			}
			b.Run(qspec.name+"-"+test.name, func(b *testing.B) {
				replies := cloneReplies(test.replies)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					qspec.spec.ReadQF(replies)
				}
			})
		}
	}
}

func cloneReplies(replies []*ReadResponse) []*ReadResponse {
	cloned := make([]*ReadResponse, len(replies))
	copy(cloned, replies)
	return cloned
}

func BenchmarkGridReadQFSuccessive(b *testing.B) {
	for _, qspec := range qspecs {
		for _, test := range gridReadQFTests {
			if !strings.Contains(test.name, "case") {
				continue
			}
			b.Run(qspec.name+"-"+test.name, func(b *testing.B) {
				replies := cloneReplies(test.replies)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for i := 0; i < len(replies); i++ {
						qspec.spec.ReadQF(replies[0 : i+1])
					}
				}
			})
		}
	}
}
