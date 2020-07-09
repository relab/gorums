package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

func generateCert() (cert, privKey []byte, err error) {
	sn, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	caTmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject: pkix.Name{
			Organization:  []string{"TEST"},
			Country:       []string{"NO"},
			Province:      []string{""},
			Locality:      []string{"Stavanger"},
			StreetAddress: []string{""},
			PostalCode:    []string{"4036"},
		},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 1, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caPK, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, caPK.Public(), caPK)
	if err != nil {
		return nil, nil, err
	}

	pkBytes, err := x509.MarshalECPrivateKey(caPK)
	if err != nil {
		return nil, nil, err
	}

	caPEM := new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	pkPEM := new(bytes.Buffer)
	err = pem.Encode(pkPEM, &pem.Block{
		Type:  "ECDSA PRIVATE KEY",
		Bytes: pkBytes,
	})
	if err != nil {
		return nil, nil, err
	}

	return caPEM.Bytes(), pkPEM.Bytes(), nil
}
