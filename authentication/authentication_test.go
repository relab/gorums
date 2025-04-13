package authentication

import (
	"crypto/elliptic"
	"errors"
	"reflect"
	"testing"
)

func TestAuthentication(t *testing.T) {
	ec := New(elliptic.P256())
	_ = ec.GenerateKeys()
	err := ec.test()
	if err != nil {
		t.Error(err)
	}
}

func TestSignAndVerify(t *testing.T) {
	ec1 := New(elliptic.P256())
	err := ec1.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}

	ec2 := New(elliptic.P256())
	err = ec2.GenerateKeys()
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
	ok, err := ec2.VerifySignature(pemEncodedPub, encodedMsg2, signature)
	if err != nil {
		t.Error(err)
	}
	if !ok {
		t.Error("signature not ok!")
	}
}

func TestVerifyWithWrongPubKey(t *testing.T) {
	ec1 := New(elliptic.P256())
	err := ec1.GenerateKeys()
	if err != nil {
		t.Fatal(err)
	}

	ec2 := New(elliptic.P256())
	err = ec2.GenerateKeys()
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
	ok, err := ec2.VerifySignature(pemEncodedPub, encodedMsg2, signature)
	if err != nil {
		t.Error(err)
	}
	if ok {
		t.Error("signature should not be ok!")
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
