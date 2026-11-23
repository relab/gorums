package certs_test

import (
	"crypto/x509"
	"net"
	"testing"

	"github.com/relab/gorums/gorumstest/certs"
)

func TestCertsLocalhost(t *testing.T) {
	c := certs.Localhost(t)
	if len(c.X509.IPAddresses) != 0 {
		t.Errorf("IPAddresses = %v, want none", c.X509.IPAddresses)
	}
	opts := x509.VerifyOptions{Roots: c.Pool(), DNSName: "localhost", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if _, err := c.X509.Verify(opts); err != nil {
		t.Errorf("Verify(localhost) = %v, want nil", err)
	}
	opts.DNSName = "127.0.0.1"
	if _, err := c.X509.Verify(opts); err == nil {
		t.Error("Verify(127.0.0.1) = nil, want error")
	}
}

func TestCertsLoopback(t *testing.T) {
	c := certs.Loopback(t)
	if len(c.X509.DNSNames) != 0 {
		t.Errorf("DNSNames = %v, want none", c.X509.DNSNames)
	}
	for _, ip := range []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback} {
		opts := x509.VerifyOptions{Roots: c.Pool(), DNSName: ip.String(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		if _, err := c.X509.Verify(opts); err != nil {
			t.Errorf("Verify(%v) = %v, want nil", ip, err)
		}
	}
}

func TestCertsIssue(t *testing.T) {
	ca := certs.NewCA(t)
	tmpl := &x509.Certificate{
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	a, b := ca.Issue(t, tmpl), ca.Issue(t, tmpl)
	if a.Key.Equal(b.Key) {
		t.Error("issued certificates share a private key")
	}
	if a.X509.SerialNumber.Cmp(b.X509.SerialNumber) == 0 {
		t.Error("issued certificates share a serial number")
	}
	for _, usage := range tmpl.ExtKeyUsage {
		opts := x509.VerifyOptions{Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{usage}}
		if _, err := a.X509.Verify(opts); err != nil {
			t.Errorf("Verify(usage %v) = %v, want nil", usage, err)
		}
	}
	if _, err := a.X509.Verify(x509.VerifyOptions{Roots: certs.NewCA(t).Pool()}); err == nil {
		t.Error("Verify against another CA = nil, want error")
	}
}

// TestCertsTemplateUsedAsGiven checks that New leaves SANs and validity
// as the template sets them, so a test can build invalid certificates.
func TestCertsTemplateUsedAsGiven(t *testing.T) {
	c := certs.New(t, &x509.Certificate{
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, nil)
	if n := len(c.X509.DNSNames) + len(c.X509.IPAddresses) + len(c.X509.URIs); n != 0 {
		t.Errorf("template without SANs produced %d SANs", n)
	}
	if c.X509.SerialNumber == nil || c.X509.NotAfter.Before(c.X509.NotBefore) {
		t.Errorf("defaults not applied: serial %v, validity %v to %v", c.X509.SerialNumber, c.X509.NotBefore, c.X509.NotAfter)
	}
}
