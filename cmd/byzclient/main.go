package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/relab/gorums/byzq"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	var (
		port     = flag.Int("port", 8080, "port where local server is listening")
		saddrs   = flag.String("addrs", "", "server addresses separated by ','")
		f        = flag.Int("f", 1, "fault tolerance, supported values f=1,2,3 (this is ignored if addrs is provided)")
		noauth   = flag.Bool("noauth", false, "don't use authenticated channels")
		generate = flag.Bool("generate", false, "generate public/private key-pair and save to file provided by -key")
		writer   = flag.Bool("writer", false, "set this client to be writer only (default is reader only)")
		keyFile  = flag.String("key", "priv-key.pem", "private key file to be used for signatures")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *generate {
		// Generate key file and exit.
		err := byzq.GenerateKeyfile(*keyFile)
		if err != nil {
			dief("error generating public/private key-pair: %v", err)
		}
		os.Exit(0)
	}

	if *saddrs == "" {
		// Use local addresses only.
		if *f > 3 || *f < 1 {
			dief("only f=1,2,3 is allowed")
		}
		n := 3**f + 1
		var buf bytes.Buffer
		for i := 0; i < n; i++ {
			buf.WriteString(":")
			buf.WriteString(strconv.Itoa(*port + i))
			buf.WriteString(",")
		}
		b := buf.String()
		*saddrs = b[:len(b)-1]
	}

	addrs := strings.Split(*saddrs, ",")
	if len(addrs) == 0 {
		dief("no server addresses provided")
	}
	log.Printf("#addrs: %d (%v)", len(addrs), *saddrs)

	var secDialOption grpc.DialOption
	if *noauth {
		secDialOption = grpc.WithInsecure()
	} else {
		// TODO: Fix hardcoded youtube server name.
		clientCreds, err := credentials.NewClientTLSFromFile("cert/ca.pem", "x.test.youtube.com")
		if err != nil {
			dief("error creating credentials: %v", err)
		}
		secDialOption = grpc.WithTransportCredentials(clientCreds)
	}

	key, err := byzq.ReadKeyfile(*keyFile)
	if err != nil {
		dief("error reading keyfile: %v", err)
	}

	mgr, err := byzq.NewManager(
		addrs,
		byzq.WithGrpcDialOptions(
			grpc.WithBlock(),
			grpc.WithTimeout(0*time.Millisecond),
			secDialOption,
		),
	)
	if err != nil {
		dief("error creating manager: %v", err)
	}
	defer mgr.Close()

	ids := mgr.NodeIDs()
	qspec, err := byzq.NewAuthDataQ(len(ids), key, &key.PublicKey)
	if err != nil {
		dief("error creating quorum specification: %v", err)
	}
	conf, err := mgr.NewConfiguration(ids, qspec)
	if err != nil {
		dief("error creating config: %v", err)
	}

	registerState := &byzq.Content{
		Key:       "Hein",
		Value:     "Meling",
		Timestamp: -1,
	}

	for {
		if *writer {
			// Writer client.
			registerState.Value = strconv.Itoa(rand.Intn(1 << 8))
			registerState.Timestamp++
			signedState, err := qspec.Sign(registerState)
			if err != nil {
				dief("failed to sign message: %v", err)
			}
			ack, err := conf.Write(context.Background(), signedState)
			if err != nil {
				dief("error writing: %v", err)
			}
			fmt.Println("WriteReturn " + ack.String())
			time.Sleep(15 * time.Second)
		} else {
			// Reader client.
			val, err := conf.Read(context.Background(), &byzq.Key{Key: registerState.Key})
			if err != nil {
				dief("error reading: %v", err)
			}
			registerState = val
			fmt.Println("ReadReturn: " + registerState.String())
			time.Sleep(10000 * time.Millisecond)
		}
	}
}

func dief(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format, a...)
	fmt.Fprint(os.Stderr, "\n")
	flag.Usage()
	os.Exit(2)
}
