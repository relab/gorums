package gbench

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	rpc "github.com/relab/gorums/byzq"
	"github.com/tylertreat/bench"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// ByzqRequesterFactory implements RequesterFactory by creating a Requester which
// issues requests to a register using the Byzq framework.
type ByzqRequesterFactory struct {
	Addrs             []string
	PayloadSize       int
	QRPCTimeout       time.Duration
	WriteRatioPercent int
}

// GetRequester returns a new Requester, called for each Benchmark connection.
func (r *ByzqRequesterFactory) GetRequester(uint64) bench.Requester {
	return &byzqRequester{
		addrs:       r.Addrs,
		payloadSize: r.PayloadSize,
		timeout:     r.QRPCTimeout,
		writeRatio:  r.WriteRatioPercent,
	}
}

type byzqRequester struct {
	addrs       []string
	payloadSize int
	timeout     time.Duration
	writeRatio  int
	keyFile     string

	mgr    *rpc.Manager
	config *rpc.Configuration
	qspec  *rpc.AuthDataQ
	state  *rpc.Content
}

func (gr *byzqRequester) Setup() error {
	var err error
	//TODO fix hardcoded youtube server name (can we get certificate for localhost servername?)
	clientCreds, err := credentials.NewClientTLSFromFile("cert/ca.pem", "x.test.youtube.com")
	if err != nil {
		return err
	}
	gr.mgr, err = rpc.NewManager(
		gr.addrs,
		rpc.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTimeout(1*time.Millisecond),
			grpc.WithTransportCredentials(clientCreds),
		),
	)
	if err != nil {
		return err
	}

	key, err := rpc.ReadKeyfile(gr.keyFile)
	if err != nil {
		return err
	}

	ids := gr.mgr.NodeIDs()
	gr.qspec, err = rpc.NewAuthDataQ(len(ids), key, &key.PublicKey)
	if err != nil {
		return err
	}
	gr.config, err = gr.mgr.NewConfiguration(ids, gr.qspec, gr.timeout)
	if err != nil {
		return err
	}

	// Set initial state.
	gr.state = &rpc.Content{
		Key:       "State",
		Value:     strings.Repeat("x", gr.payloadSize),
		Timestamp: time.Now().UnixNano(),
	}
	// Sign initial state
	signedState, err := gr.qspec.Sign(gr.state)
	if err != nil {
		return err
	}
	ack, err := gr.config.Write(signedState)
	if err != nil {
		return fmt.Errorf("write rpc error: %v", err)
	}
	if ack.Reply.Timestamp == 0 {
		return fmt.Errorf("intital write reply was not marked as new")
	}
	return nil
}

func (gr *byzqRequester) Request() error {
	var err error
	switch gr.writeRatio {
	case 0:
		_, err = gr.config.Read(&rpc.Key{})
	case 100:
		gr.state.Timestamp = time.Now().UnixNano()
		signedState, err := gr.qspec.Sign(gr.state)
		if err != nil {
			return err
		}
		_, err = gr.config.Write(signedState)
	default:
		x := rand.Intn(100)
		if x < gr.writeRatio {
			gr.state.Timestamp = time.Now().UnixNano()
			signedState, err := gr.qspec.Sign(gr.state)
			if err != nil {
				return err
			}
			_, err = gr.config.Write(signedState)
		} else {
			_, err = gr.config.Read(&rpc.Key{})
		}
	}

	return err
}

func (gr *byzqRequester) Teardown() error {
	gr.mgr.Close()
	gr.mgr = nil
	gr.config = nil
	return nil
}
