package authentication

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
)

// Elliptic Curve Cryptography (ECC) is a key-based technique for encrypting data.
// ECC focuses on pairs of public and private keys for decryption and encryption of web traffic.
// ECC is frequently discussed in the context of the Rivest–Shamir–Adleman (RSA) cryptographic algorithm.
//
// https://pkg.go.dev/github.com/katzenpost/core/crypto/eddsa
type EllipticCurve struct {
	addr        net.Addr       // used to identify self
	pubKeyCurve elliptic.Curve // http://golang.org/pkg/crypto/elliptic/#P256
	privateKey  *ecdsa.PrivateKey
	publicKey   *ecdsa.PublicKey
}

// New EllipticCurve instance
func New(curve elliptic.Curve) *EllipticCurve {
	return &EllipticCurve{
		pubKeyCurve: curve,
		privateKey:  new(ecdsa.PrivateKey),
	}
}

// GenerateKeys EllipticCurve public and private keys
func (ec *EllipticCurve) GenerateKeys() error {
	privKey, err := ecdsa.GenerateKey(ec.pubKeyCurve, rand.Reader)
	if err != nil {
		return err
	}
	ec.privateKey = privKey
	ec.publicKey = &privKey.PublicKey
	return nil
}

// RegisterKeys EllipticCurve public and private keys
func (ec *EllipticCurve) RegisterKeys(addr net.Addr, privKey *ecdsa.PrivateKey, pubKey *ecdsa.PublicKey) {
	ec.addr = addr
	ec.privateKey = privKey
	ec.publicKey = pubKey
}

// Returns the EllipticCurve public and private keys
func (ec *EllipticCurve) Keys() (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	return ec.privateKey, ec.publicKey
}

// Returns the address
func (ec *EllipticCurve) Addr() string {
	return ec.addr.String()
}

// EncodePrivate private key
func (ec *EllipticCurve) EncodePrivate() (string, error) {
	encoded, err := x509.MarshalECPrivateKey(ec.privateKey)
	if err != nil {
		return "", err
	}
	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encoded})
	key := string(pemEncoded)
	return key, nil
}

// EncodePublic public key
func (ec *EllipticCurve) EncodePublic() (string, error) {
	encoded, err := x509.MarshalPKIXPublicKey(ec.publicKey)
	if err != nil {
		return "", err
	}
	pemEncodedPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encoded})
	key := string(pemEncodedPub)
	return key, nil
}

// DecodePrivate private key
func (ec *EllipticCurve) DecodePrivate(pemEncodedPriv string) (*ecdsa.PrivateKey, error) {
	blockPriv, _ := pem.Decode([]byte(pemEncodedPriv))
	x509EncodedPriv := blockPriv.Bytes
	return x509.ParseECPrivateKey(x509EncodedPriv)
}

// DecodePublic public key
func (ec *EllipticCurve) DecodePublic(pemEncodedPub string) (*ecdsa.PublicKey, error) {
	blockPub, _ := pem.Decode([]byte(pemEncodedPub))
	if blockPub == nil {
		return nil, fmt.Errorf("invalid publicKey")
	}
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, err := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)
	return publicKey, err
}

func (ec *EllipticCurve) Sign(msg []byte) ([]byte, error) {
	h := sha256.Sum256(msg)
	hash := h[:]
	signature, err := ecdsa.SignASN1(rand.Reader, ec.privateKey, hash)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

func (ec *EllipticCurve) Hash(msg []byte) []byte {
	h := sha256.Sum256(msg)
	hash := h[:]
	return hash
}

// VerifySignature sign ecdsa style and verify signature
func (ec *EllipticCurve) VerifySignature(pemEncodedPub string, msg, signature []byte) (bool, error) {
	h := sha256.Sum256(msg)
	hash := h[:]
	pubKey, err := ec.DecodePublic(pemEncodedPub)
	if err != nil {
		return false, err
	}
	ok := ecdsa.VerifyASN1(pubKey, hash, signature)
	return ok, nil
}

func (ec *EllipticCurve) EncodeMsg(msg any) ([]byte, error) {
	return []byte(fmt.Sprintf("%v", msg)), nil
	/*var encodedMsg bytes.Buffer
	gob.Register(msg)
	enc := gob.NewEncoder(&encodedMsg)
	err := enc.Encode(msg)
	if err != nil {
		return nil, err
	}
	return encodedMsg.Bytes(), nil*/
}

func encodeMsg(msg any) ([]byte, error) {
	return []byte(fmt.Sprintf("%v", msg)), nil
}

func Verify(pemEncodedPub string, signature, digest []byte, msg any) (bool, error) {
	encodedMsg, err := encodeMsg(msg)
	if err != nil {
		return false, err
	}
	ec := New(elliptic.P256())
	h := sha256.Sum256(encodedMsg)
	hash := h[:]
	if !bytes.Equal(hash, digest) {
		return false, fmt.Errorf("wrong digest")
	}
	pubKey, err := ec.DecodePublic(pemEncodedPub)
	if err != nil {
		return false, err
	}
	ok := ecdsa.VerifyASN1(pubKey, hash, signature)
	return ok, nil
}
