package gorums

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"google.golang.org/grpc/encoding"
)

var (
	nodes   = []string{"127.0.0.1:9080", "127.0.0.1:9081", "127.0.0.1:9082"}
	nodeMap = map[string]uint32{"127.0.0.1:9080": 1, "127.0.0.1:9081": 2, "127.0.0.1:9082": 3, "127.0.0.1:9083": 4}
)

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
	buf.WriteString("\n")
	_ = NewManager(WithNoConnect(), WithLogger(logger))
	want := "logger: mgr.go:48: ready"
	if strings.TrimSpace(buf.String()) != want {
		t.Errorf("logger: got %q, want %q", buf.String(), want)
	}
}

func TestManagerAddNode(t *testing.T) {
	mgr := NewManager(WithNoConnect())
	_, err := NewConfiguration(mgr, WithNodeMap(nodeMap))
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
		node, err := NewNodeWithID(test.addr, test.id)
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.addNode(node); err != nil {
			if err.Error() == test.err {
				continue
			}
			t.Errorf("mgr.addNode(%s, %d) = %q, want %q", test.addr, test.id, err.Error(), test.err)
		}
	}
}

func TestManagerAddNodeWithConn(t *testing.T) {
	addrs := TestServers(t, 3, nil)
	mgr := NewManager(InsecureDialOptions(t))
	t.Cleanup(mgr.Close)

	// Create configuration with only first 2 nodes
	_, err := NewConfiguration(mgr, WithNodeList(addrs[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if mgr.Size() != len(addrs)-1 {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs)-1)
	}

	// Add the 3rd node to the manager
	node, err := NewNode(addrs[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.addNode(node); err != nil {
		t.Errorf("mgr.addNode(%s) = %q, expected no error", addrs[2], err.Error())
	}
	if mgr.Size() != len(addrs) {
		t.Errorf("mgr.Size() = %d, expected %d", mgr.Size(), len(addrs))
	}
}
