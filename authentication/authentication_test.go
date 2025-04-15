package authentication

import (
	"crypto/elliptic"
	"errors"
	"net"
	"reflect"
	"testing"
)

func TestAuthentication(t *testing.T) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	ec, err := NewWithAddr(elliptic.P256(), addr)
	if err != nil {
		t.Fatal(err)
	}
	err = ec.test()
	if err != nil {
		t.Error(err)
	}
}

func TestSignAndVerify(t *testing.T) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	ec1, err := NewWithAddr(elliptic.P256(), addr)
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := NewWithAddr(elliptic.P256(), addr)
	if err != nil {
		t.Fatal(err)
	}

	message := "This is a message"
	encodedMsg1 := EncodeMsg(message)
	signature, err := ec1.Sign(encodedMsg1)
	if err != nil {
		t.Error(err)
	}
	pemEncodedPub, err := ec1.EncodePublic()
	if err != nil {
		t.Error(err)
	}

	encodedMsg2 := EncodeMsg(message)
	err = ec2.VerifySignature(pemEncodedPub, encodedMsg2, signature)
	if err != nil {
		t.Errorf("VerifySignature() = %v, want nil", err)
	}
}

func TestVerifyWithWrongPubKey(t *testing.T) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:5000")
	if err != nil {
		t.Fatal(err)
	}
	ec1, err := NewWithAddr(elliptic.P256(), addr)
	if err != nil {
		t.Fatal(err)
	}
	ec2, err := NewWithAddr(elliptic.P256(), addr)
	if err != nil {
		t.Fatal(err)
	}

	message := "This is a message"
	encodedMsg1 := EncodeMsg(message)
	signature, err := ec1.Sign(encodedMsg1)
	if err != nil {
		t.Error(err)
	}

	// encoding ec2 public key instead of ec1 (which was used in signing)
	pemEncodedPub, err := ec2.EncodePublic()
	if err != nil {
		t.Error(err)
	}

	encodedMsg2 := EncodeMsg(message)
	err = ec2.VerifySignature(pemEncodedPub, encodedMsg2, signature)
	if err == nil {
		t.Errorf("VerifySignature() = nil, want %v", InvalidSignatureErr)
	} else {
		if !errors.Is(err, InvalidSignatureErr) {
			t.Errorf("VerifySignature() = %v, want %v", err, InvalidSignatureErr)
		}
	}
}

// Can be used in _test.go
// Test encode, decode and test it with deep equal
func (ec *EllipticCurve) test() error {
	encPriv, err := ec.EncodePrivate()
	if err != nil {
		return err
	}
	encPub, err := ec.EncodePublic()
	if err != nil {
		return err
	}
	priv2, err := ec.DecodePrivate(encPriv)
	if err != nil {
		return err
	}
	pub2, err := ec.DecodePublic(encPub)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(ec.privateKey, priv2) {
		return errors.New("private keys do not match")
	}
	if !reflect.DeepEqual(ec.publicKey, pub2) {
		return errors.New("public keys do not match")
	}
	return nil
}
