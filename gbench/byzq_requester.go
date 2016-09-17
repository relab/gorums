package gbench

import (
	"crypto/tls"
	"crypto/x509"
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

	mgr    *rpc.Manager
	config *rpc.Configuration
	qspec  *rpc.AuthDataQ
	state  *rpc.Content
}

const certPEM = `-----BEGIN CERTIFICATE-----
MIICSjCCAbOgAwIBAgIJAJHGGR4dGioHMA0GCSqGSIb3DQEBCwUAMFYxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxDzANBgNVBAMTBnRlc3RjYTAeFw0xNDExMTEyMjMxMjla
Fw0yNDExMDgyMjMxMjlaMFYxCzAJBgNVBAYTAkFVMRMwEQYDVQQIEwpTb21lLVN0
YXRlMSEwHwYDVQQKExhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQxDzANBgNVBAMT
BnRlc3RjYTCBnzANBgkqhkiG9w0BAQEFAAOBjQAwgYkCgYEAwEDfBV5MYdlHVHJ7
+L4nxrZy7mBfAVXpOc5vMYztssUI7mL2/iYujiIXM+weZYNTEpLdjyJdu7R5gGUu
g1jSVK/EPHfc74O7AyZU34PNIP4Sh33N+/A5YexrNgJlPY+E3GdVYi4ldWJjgkAd
Qah2PH5ACLrIIC6tRka9hcaBlIECAwEAAaMgMB4wDAYDVR0TBAUwAwEB/zAOBgNV
HQ8BAf8EBAMCAgQwDQYJKoZIhvcNAQELBQADgYEAHzC7jdYlzAVmddi/gdAeKPau
sPBG/C2HCWqHzpCUHcKuvMzDVkY/MP2o6JIW2DBbY64bO/FceExhjcykgaYtCH/m
oIU63+CFOTtR7otyQAWHqXa7q4SbCDlG7DyRFxqG0txPtGvy12lgldA2+RgcigQG
Dfcog5wrJytaQ6UA0wE=
-----END CERTIFICATE-----`

const keyPEM = `-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIANyDBAupB6O86ORJ1u95Cz6C+lz3x2WKOFntJNIesvioAoGCCqGSM49
AwEHoUQDQgAE+pBXRIe0CI3vcdJwSvU37RoTqlPqEve3fcC36f0pY/X9c9CsgkFK
/sHuBztq9TlUfC0REC81NRqRgs6DTYJ/4Q==
-----END EC PRIVATE KEY-----`

func (gr *byzqRequester) Setup() error {
	var err error
	//TODO fix hardcoded youtube server name (can we get certificate for localhost servername?)
	cp := x509.NewCertPool()
	if !cp.AppendCertsFromPEM([]byte(certPEM)) {
		return fmt.Errorf("credentials: failed to append certificates")
	}
	clientCreds := credentials.NewTLS(&tls.Config{ServerName: "x.test.youtube.com", RootCAs: cp})

	// clientCreds, err := credentials.NewClientTLSFromFile("cert/ca.pem", "x.test.youtube.com")
	// if err != nil {
	// 	return err
	// }
	gr.mgr, err = rpc.NewManager(
		gr.addrs,
		rpc.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTimeout(time.Second),
			grpc.WithTransportCredentials(clientCreds),
		),
	)
	if err != nil {
		return err
	}

	key, err := rpc.ParseKey(keyPEM)
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
		signedState, err2 := gr.qspec.Sign(gr.state)
		if err2 != nil {
			return err2
		}
		_, err = gr.config.Write(signedState)
	default:
		x := rand.Intn(100)
		if x < gr.writeRatio {
			gr.state.Timestamp = time.Now().UnixNano()
			signedState, err2 := gr.qspec.Sign(gr.state)
			if err2 != nil {
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
