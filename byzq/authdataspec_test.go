package byzq

import (
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

// TODO(meling): Make tests for f=2 and f=3.

var priv *ecdsa.PrivateKey

var pemKeyData = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIANyDBAupB6O86ORJ1u95Cz6C+lz3x2WKOFntJNIesvioAoGCCqGSM49
AwEHoUQDQgAE+pBXRIe0CI3vcdJwSvU37RoTqlPqEve3fcC36f0pY/X9c9CsgkFK
/sHuBztq9TlUfC0REC81NRqRgs6DTYJ/4Q==
-----END EC PRIVATE KEY-----`

func TestMain(m *testing.M) {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false
	var err error
	priv, err = ParseKey(pemKeyData)
	if err != nil {
		log.Fatalln("couldn't parse private key")
	}
	res := m.Run()
	os.Exit(res)
}

var authQTests = []struct {
	n   int
	f   int // expected value
	q   int // expected value
	err string
}{
	{-1, 0, 1, "Byzantine quorum require n>3f replicas; only got n=-1, yielding f=0"},
	{0, 0, 1, "Byzantine quorum require n>3f replicas; only got n=0, yielding f=0"},
	{1, 0, 1, "Byzantine quorum require n>3f replicas; only got n=1, yielding f=0"},
	{2, 0, 2, "Byzantine quorum require n>3f replicas; only got n=2, yielding f=0"},
	{3, 0, 2, "Byzantine quorum require n>3f replicas; only got n=3, yielding f=0"},
	{4, 1, 2, ""},
	{5, 1, 3, ""},
	{6, 1, 3, ""},
	{7, 2, 4, ""},
	{8, 2, 5, ""},
	{9, 2, 5, ""},
	{10, 3, 6, ""},
	{11, 3, 7, ""},
	{12, 3, 7, ""},
	{13, 4, 8, ""},
	{14, 4, 9, ""},
}

func TestNewAuthDataQ(t *testing.T) {
	for _, test := range authQTests {
		bq, err := NewAuthDataQ(test.n, priv, &priv.PublicKey)
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

var (
	myContent = &Content{Key: "Winnie", Value: "Poo", Timestamp: 3}
	myVal     = &Value{C: myContent}
)

var authReadQFTests = []struct {
	name     string
	replies  []*Value
	expected *Content
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
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
		},
		nil,
		false,
	},
	{
		"no quorum (II)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
		},
		nil,
		false,
	},
	{
		"quorum (I)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
		},
		&Content{Key: "Winnie", Value: "Poop", Timestamp: 1},
		true,
	},
	{
		"quorum (II)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 2}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
		},
		&Content{Key: "Winnie", Value: "Poo", Timestamp: 2},
		true,
	},
	{
		"quorum (III)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 2}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Pooop", Timestamp: 3}},
		},
		&Content{Key: "Winnie", Value: "Pooop", Timestamp: 3},
		true,
	},
	{
		"quorum (IV)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 2}},
			{C: &Content{Key: "Winnie", Value: "Poop", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Pooop", Timestamp: 3}},
			{C: &Content{Key: "Winnie", Value: "Poooop", Timestamp: 4}},
		},
		&Content{Key: "Winnie", Value: "Poooop", Timestamp: 4},
		true,
	},
	{
		"quorum (V)",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myContent,
		true,
	},
	{
		"quorum (VI)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			myVal,
			myVal,
			myVal,
		},
		myContent,
		true,
	},
	{
		"quorum (VII)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			myVal,
			myVal,
		},
		myContent,
		true,
	},
	{
		"quorum (VIII)",
		[]*Value{
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			{C: &Content{Key: "Winnie", Value: "Poo", Timestamp: 1}},
			myVal,
		},
		myContent,
		true,
	},
	{
		"bast-case quorum",
		[]*Value{
			myVal,
			myVal,
			myVal,
		},
		myContent,
		true,
	},
	{
		"worst-case quorum",
		[]*Value{
			myVal,
			myVal,
			myVal,
			myVal,
		},
		myContent,
		true,
	},
}

func TestAuthDataQ(t *testing.T) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		t.Error(err)
	}
	for _, test := range authReadQFTests {
		for i, r := range test.replies {
			test.replies[i], err = qspec.Sign(r.C)
			if err != nil {
				t.Fatal("Failed to sign message")
			}
		}

		t.Run(fmt.Sprintf("(NoSignVerification)ReadQF(4,1) %s", test.name), func(t *testing.T) {
			reply, byzquorum := qspec.ReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if reply != nil {
				if !reply.Equal(test.expected) {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			} else {
				if test.expected != nil {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			}
		})

		t.Run(fmt.Sprintf("SequentialVerifyReadQFReadQF(4,1) %s", test.name), func(t *testing.T) {
			reply, byzquorum := qspec.SequentialVerifyReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if reply != nil {
				if !reply.Equal(test.expected) {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			} else {
				if test.expected != nil {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			}
		})

		t.Run(fmt.Sprintf("ConcurrentVerifyIndexChanReadQF(4,1) %s", test.name), func(t *testing.T) {
			reply, byzquorum := qspec.ConcurrentVerifyIndexChanReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if reply != nil {
				if !reply.Equal(test.expected) {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			} else {
				if test.expected != nil {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			}
		})
		t.Run(fmt.Sprintf("VerfiyLastReplyFirstReadQF(4,1) %s", test.name), func(t *testing.T) {
			reply, byzquorum := qspec.VerfiyLastReplyFirstReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if reply != nil {
				if !reply.Equal(test.expected) {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			} else {
				if test.expected != nil {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			}
		})

		t.Run(fmt.Sprintf("ConcurrentVerifyWGReadQF(4,1) %s", test.name), func(t *testing.T) {
			reply, byzquorum := qspec.ConcurrentVerifyWGReadQF(test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if reply != nil {
				if !reply.Equal(test.expected) {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			} else {
				if test.expected != nil {
					t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
				}
			}
		})
	}
}

func BenchmarkAuthDataQ(b *testing.B) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		b.Error(err)
	}
	for _, test := range authReadQFTests {
		if !strings.Contains(test.name, "case") {
			continue
		}
		for i, r := range test.replies {
			test.replies[i], err = qspec.Sign(r.C)
			if err != nil {
				b.Fatal("Failed to sign message")
			}
		}

		b.Run(fmt.Sprintf("(NoSignVerification)ReadQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.ReadQF(test.replies)
			}
		})

		b.Run(fmt.Sprintf("SequentialVerifyReadQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.SequentialVerifyReadQF(test.replies)
			}
		})

		b.Run(fmt.Sprintf("ConcurrentVerifyIndexChanReadQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.ConcurrentVerifyIndexChanReadQF(test.replies)
			}
		})
		b.Run(fmt.Sprintf("VerfiyLastReplyFirstReadQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.VerfiyLastReplyFirstReadQF(test.replies)
			}
		})

		b.Run(fmt.Sprintf("ConcurrentVerifyWGReadQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.ConcurrentVerifyWGReadQF(test.replies)
			}
		})
	}
}

var authWriteQFTests = []struct {
	name     string
	replies  []*WriteResponse
	expected *WriteResponse
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
		[]*WriteResponse{},
		nil,
		false,
	},
	{
		"no quorum (I)",
		[]*WriteResponse{
			{Timestamp: 1},
		},
		nil,
		false,
	},
	{
		"no quorum (II)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
		},
		nil,
		false,
	},
	{
		"no quorum (III)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 2},
			{Timestamp: 3},
			{Timestamp: 4},
		},
		nil,
		false,
	},
	{
		"no quorum (IV)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 2},
			{Timestamp: 2},
		},
		nil,
		false,
	},
	{
		"quorum (I)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (II)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (III)",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 2},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"quorum (IV)",
		[]*WriteResponse{
			{Timestamp: 2},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"best-case quorum",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
	{
		"worst-case quorum",
		[]*WriteResponse{
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
			{Timestamp: 1},
		},
		&WriteResponse{Timestamp: 1},
		true,
	},
}

func TestAuthDataQW(t *testing.T) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		t.Error(err)
	}
	for _, test := range authWriteQFTests {
		t.Run(fmt.Sprintf("WriteQF(4,1) %s", test.name), func(t *testing.T) {
			req := &Value{C: &Content{Timestamp: 0}}
			if test.expected != nil {
				req = &Value{C: &Content{Timestamp: test.expected.Timestamp}}
			}
			reply, byzquorum := qspec.WriteQF(req, test.replies)
			if byzquorum != test.rq {
				t.Errorf("got %t, want %t", byzquorum, test.rq)
			}
			if !reply.Equal(test.expected) {
				t.Errorf("got %v, want %v as quorum reply", reply, test.expected)
			}
		})
	}
}

func BenchmarkAuthDataQW(b *testing.B) {
	qspec, err := NewAuthDataQ(4, priv, &priv.PublicKey)
	if err != nil {
		b.Error(err)
	}
	for _, test := range authWriteQFTests {
		req := &Value{C: &Content{Timestamp: 0}}
		if test.expected != nil {
			req = &Value{C: &Content{Timestamp: test.expected.Timestamp}}
		}
		if !strings.Contains(test.name, "case") {
			continue
		}
		b.Run(fmt.Sprintf("WriteQF(4,1) %s", test.name), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				qspec.WriteQF(req, test.replies)
			}
		})
	}
}
