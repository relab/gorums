package byzq

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
)

// See https://golang.org/src/crypto/tls/generate_cert.go

var curve = elliptic.P256()

// ParseKey takes a PEM formatted string and returns a private key.
func ParseKey(pemKey string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("no block to decode")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse key from pem block: %v\n %v", err, key)
	}
	return key, nil
}

// ReadKeyfile reads the provided keyFile and returns the private key
// stored in the file.
func ReadKeyfile(keyFile string) (*ecdsa.PrivateKey, error) {
	b, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	return ParseKey(string(b))
}

// GenerateKeyfile generates a private key and writes it to the given keyFile.
// Note that if the file exists it will be overwritten.
func GenerateKeyfile(keyFile string) error {
	f, err := os.OpenFile(keyFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	key, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return err
	}

	ec, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("failed to marshal key from pem block: %v\n %v", err, key)
	}

	err = pem.Encode(f, &pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: ec,
	})
	if err != nil {
		return fmt.Errorf("failed to PEM encode key: %v\n %v", err, key)
	}
	return nil
}
