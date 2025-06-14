package authentication

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
)

// EllipticCurve represents an elliptic curve instance used for message authentication.
type EllipticCurve struct {
	addr       net.Addr // used to identify self
	curve      elliptic.Curve
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
}

// New EllipticCurve instance
func New(curve elliptic.Curve) *EllipticCurve {
	return &EllipticCurve{
		curve:      curve,
		privateKey: new(ecdsa.PrivateKey),
	}
}

// NewWithAddr returns a new EllipticCurve instance with the given address.
// It generates a new public-private key pair for the specified elliptic curve.
func NewWithAddr(curve elliptic.Curve, addr net.Addr) (*EllipticCurve, error) {
	if addr == nil {
		return nil, errors.New("address cannot be nil")
	}
	priv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}
	return &EllipticCurve{
		addr:       addr,
		curve:      curve,
		privateKey: priv,
		publicKey:  &priv.PublicKey,
	}, nil
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
		return nil, errors.New("invalid public key")
	}
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, err := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)
	return publicKey, err
}

func (ec *EllipticCurve) Sign(msg []byte) ([]byte, error) {
	return ecdsa.SignASN1(rand.Reader, ec.privateKey, Hash(msg))
}

func Hash(msg []byte) []byte {
	hash := sha256.Sum256(msg)
	return hash[:]
}

var InvalidSignatureErr = errors.New("invalid signature")

// VerifySignature verifies the signature of the message's hash using the given PEM encoded
// public key. It returns an error if the signature is invalid or if there is an error
// decoding the public key.
func (ec *EllipticCurve) VerifySignature(pemEncodedPub string, msg, signature []byte) error {
	pubKey, err := ec.DecodePublic(pemEncodedPub)
	if err != nil {
		return err
	}
	if valid := ecdsa.VerifyASN1(pubKey, Hash(msg), signature); !valid {
		return InvalidSignatureErr
	}
	return nil
}

func EncodeMsg(msg any) []byte {
	return fmt.Appendf(nil, "%v", msg)
}

func Verify(pemEncodedPub string, signature, digest []byte, msg any) error {
	encodedMsg := EncodeMsg(msg)
	ec := New(elliptic.P256())
	hash := Hash(encodedMsg)
	if !bytes.Equal(hash, digest) {
		return errors.New("invalid digest for message")
	}
	return ec.VerifySignature(pemEncodedPub, encodedMsg, signature)
}
