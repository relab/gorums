package gorums

/*import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestBroadcastFunc(t *testing.T) {
	srv := newBroadcastServer(nil)
	srv.registerBroadcastFunc("test")
	listener, err := net.Listen("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Error(err)
	}
	srv.addAddr(listener)
	req := newMessage(requestType)
	var mut sync.Mutex // used to achieve mutex between request handlers
	finished := make(chan *Message, 10)
	clientHandlerMock := func(broadcastID string, req protoreflect.ProtoMessage, cc *grpc.ClientConn, timeout time.Duration, opts ...grpc.CallOption) (any, error) {
		return nil, nil
	}
	srv.registerSendToClientHandler("client", clientHandlerMock)
	ctx := context.Background()
	mut.Lock()
	handler := BroadcastHandler2()
	go handler(ServerCtx{Context: ctx, once: new(sync.Once), mut: &mut}, req, finished)
}
*/
