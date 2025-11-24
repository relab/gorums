package gorums

import (
	"os"
	"testing"

	"github.com/relab/gorums/internal/testutils/mock"
	pb "google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMain(m *testing.M) {
	// Register the default mock services for integration tests.
	if err := mock.RegisterServices([]mock.Service{
		{
			Name: "mock.MockService",
			Methods: []mock.Method{
				{
					Name:   "Test",
					Input:  &pb.StringValue{},
					Output: &pb.StringValue{},
				},
				{
					Name:   "GetValue",
					Input:  &pb.Int32Value{},
					Output: &pb.Int32Value{},
				},
			},
		},
	}); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}
