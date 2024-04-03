package authentication

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/x509"
	"encoding/gob"
	"encoding/pem"
	"io"
)

// Elliptic Curve Cryptography (ECC) is a key-based technique for encrypting data.
// ECC focuses on pairs of public and private keys for decryption and encryption of web traffic.
// ECC is frequently discussed in the context of the Rivest–Shamir–Adleman (RSA) cryptographic algorithm.
// EllipticCurve data struct
type EllipticCurve struct {
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
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, err := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*ecdsa.PublicKey)
	return publicKey, err
}

func (ec *EllipticCurve) Sign(msg []byte) ([]byte, error) {
	h := md5.New()
	_, err := h.Write(msg)
	if err != nil {
		return nil, err
	}
	hash := h.Sum(nil)
	signature, err := ecdsa.SignASN1(rand.Reader, ec.privateKey, hash)
	if err != nil {
		return nil, err
	}
	return signature, nil
}

// VerifySignature sign ecdsa style and verify signature
func (ec *EllipticCurve) VerifySignature(pemEncodedPub string, msg, signature []byte) (bool, error) {
	h := md5.New()
	_, err := h.Write(msg)
	if err != nil {
		return false, err
	}
	hash := h.Sum(nil)
	pubKey, err := ec.DecodePublic(pemEncodedPub)
	if err != nil {
		return false, err
	}
	ok := ecdsa.VerifyASN1(pubKey, hash, signature)
	return ok, nil
}

// VerifySignature sign ecdsa style and verify signature
func (ec *EllipticCurve) SignAndVerify(privKey *ecdsa.PrivateKey, pubKey *ecdsa.PublicKey) ([]byte, bool, error) {
	h := md5.New()
	_, err := io.WriteString(h, "This is a message to be signed and verified by ECDSA!")
	if err != nil {
		return nil, false, err
	}
	hash := h.Sum(nil)
	signature, err := ecdsa.SignASN1(rand.Reader, privKey, hash)
	if err != nil {
		return nil, false, err
	}
	ok := ecdsa.VerifyASN1(pubKey, hash, signature)
	return signature, ok, nil
}

func (ec *EllipticCurve) EncodeMsg(msg any) ([]byte, error) {
	var encodedMsg bytes.Buffer
	enc := gob.NewEncoder(&encodedMsg)
	err := enc.Encode(msg)
	if err != nil {
		return nil, err
	}
	return encodedMsg.Bytes(), nil
}
