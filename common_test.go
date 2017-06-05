package gorums

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
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

func runAndCaptureOutput(command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%v\n%s", err, out)
	}
	return bytes.TrimSuffix(out, []byte{'\n'}), nil
}

const (
	protocVersionPrefix  = "libprotoc "
	currentProtocVersion = "3.3.1"
)

func checkProtocVersion(t *testing.T) {
	out, err := runAndCaptureOutput("protoc", "--version")
	if err != nil {
		t.Skipf("skipping test due to protoc error: %v", err)
	}
	gotVersion := string(out)
	gotVersion = strings.TrimPrefix(gotVersion, protocVersionPrefix)
	if gotVersion != currentProtocVersion {
		t.Skipf("skipping test due to old protoc version, got %q, required is %q", gotVersion, currentProtocVersion)
	}
}
