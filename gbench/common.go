package gbench

import (
	"io/ioutil"
	"log"
	"math/rand"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
)

func init() {
	silentLogger := log.New(ioutil.Discard, "", log.LstdFlags)
	grpclog.SetLogger(silentLogger)
	grpc.EnableTracing = false
	rand.Seed(42)
}
