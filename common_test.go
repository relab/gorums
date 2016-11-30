package gorums

import (
	"os"
	"os/exec"
	"testing"
)

const (
	gorumsBaseImport  = "github.com/relab/gorums"
	devImport         = gorumsBaseImport + "/" + devFolder
	testdataDevImport = gorumsBaseImport + "/" + testdataFolder + "/" + devFolder

	devFolder       = "dev"
	testdataFolder  = "testdata"
	regGoldenFolder = "register_golden"

	regProtoFile = "register.proto"
	regPbGoFile  = "register.pb.go"

	devRegProtoRelPath = devFolder + "/" + regProtoFile
	tdRegProtoRelPath  = testdataFolder + "/" + regGoldenFolder + "/" + regProtoFile
	tdRegPbGoRelPath   = testdataFolder + "/" + regGoldenFolder + "/" + regPbGoFile

	protoc        = "protoc"
	protocIFlag   = "-I=../../../:."
	protocOutFlag = "--gorums_out=plugins=grpc+gorums:"
)

func run(t *testing.T, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
}
