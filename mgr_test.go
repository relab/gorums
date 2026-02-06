package gorums

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"google.golang.org/grpc/encoding"
)

var nodeMap = map[uint32]testAddr{1: {"127.0.0.1:9080"}, 2: {"127.0.0.1:9081"}, 3: {"127.0.0.1:9082"}, 4: {"127.0.0.1:9083"}}

type testAddr struct{ addr string }

func (t testAddr) Addr() string { return t.addr }

func init() {
	if encoding.GetCodec(ContentSubtype) == nil {
		encoding.RegisterCodec(NewCodec())
	}
}

func TestManagerLogging(t *testing.T) {
	var (
		buf    bytes.Buffer
		logger = log.New(&buf, "logger: ", log.Lshortfile)
	)
	mgr := NewManager(InsecureDialOptions(t), WithLogger(logger))
	t.Cleanup(Closer(t, mgr))

	want := "logger: mgr.go:49: ready"
	if strings.TrimSpace(buf.String()) != want {
		t.Errorf("logger: got %q, want %q", buf.String(), want)
	}
}

func TestManagerNewNode(t *testing.T) {
	mgr := NewManager(InsecureDialOptions(t))
	t.Cleanup(Closer(t, mgr))

	_, err := NewConfiguration(mgr, WithNodes(nodeMap))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		addr string
		id   uint32
		err  string
	}{
		{"127.0.1.1:1234", 4, "node 4 already exists"},
		{"127.0.1.1:1234", 5, ""},
		{"127.0.1.1:1234", 6, ""}, // The same addr:port can have different IDs
	}
	for _, test := range tests {
		_, err := mgr.newNode(test.addr, test.id)
		if err != nil {
			if err.Error() == test.err {
				continue
			}
			t.Errorf("mgr.addNode(%s, %d) = %q, want %q", test.addr, test.id, err.Error(), test.err)
		}
	}
}
