package conn

import (
	"crypto/x509"
	"net/url"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func TestPeerVerifierProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		label := rapid.StringMatching(`[a-z][a-z0-9]{0,12}`).Draw(t, "label")
		uri := rapid.Bool().Draw(t, "uri")
		changeCase := rapid.Bool().Draw(t, "changeCase")
		include := rapid.Bool().Draw(t, "includeSAN")
		verified := rapid.Bool().Draw(t, "verified")
		id := rapid.Uint32Range(1, 100).Draw(t, "id")
		name := "node-" + label + ".example"
		if uri {
			name = "spiffe://cluster/node/" + label
		}
		san := name
		if changeCase {
			san = strings.ToUpper(san)
		}
		verify, err := NewPeerVerifier(identityNodes(map[ID]string{id: name}))
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{DNSNames: []string{"decoy.example"}}
		if include {
			if uri {
				u, e := url.Parse(san)
				if e != nil {
					t.Fatal(e)
				}
				cert.URIs = []*url.URL{u}
			} else {
				cert.DNSNames = append(cert.DNSNames, san)
			}
		}
		ctx := tlsPeerContext(cert, verified)
		want := verified && include && (!uri || !changeCase)
		if err := verify(ctx, id); (err == nil) != want {
			t.Fatalf("verify(%q, SAN %q) = %v, accept=%v", name, san, err, want)
		}
		if err := verify(ctx, id+1); err == nil {
			t.Fatal("accepted unconfigured ID")
		}
		// Two identities are ambiguous precisely when the same SAN can match both.
		_, err = NewPeerVerifier(identityNodes(map[ID]string{id: name, id + 1: san}))
		duplicate := !uri || !changeCase
		if (err != nil) != duplicate {
			t.Fatalf("duplicate identity error = %v, duplicate=%v", err, duplicate)
		}
	})
}

func FuzzPeerIdentity(f *testing.F) {
	for _, s := range []string{"", "node.example", "Node.Example", "spiffe://cluster/node/1", "SPIFFE://cluster/node/1", "urn:node:1", "*.example", "127.0.0.1", "[::1]:9000", "host:9000", "node..example", "\x00", "é.example"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, identity string) {
		if len(identity) > 4096 {
			t.Skip()
		}
		parsed, err := parseIdentity(identity)
		verify, verifierErr := NewPeerVerifier(identityNodes(map[ID]string{1: identity}))
		if err != nil {
			if verifierErr == nil {
				t.Fatal("verifier accepted invalid identity")
			}
			return
		}
		if verifierErr != nil {
			t.Fatalf("valid identity rejected: %v", verifierErr)
		}
		for _, c := range []byte(identity) {
			if c <= ' ' || c > '~' {
				t.Fatal("accepted non-printable identity")
			}
		}
		again, err := parseIdentity(parsed.name)
		if err != nil || again != parsed {
			t.Fatalf("identity normalization is unstable: %v, %v", again, err)
		}
		cert := new(x509.Certificate)
		if parsed.uri {
			u, err := url.Parse(parsed.name)
			if err != nil || u.Scheme == "" {
				t.Fatalf("invalid accepted URI: %v", err)
			}
			cert.URIs = []*url.URL{u}
		} else {
			if strings.Contains(parsed.name, "*") {
				t.Fatal("accepted wildcard DNS identity")
			}
			cert.DNSNames = []string{parsed.name}
		}
		if err := verify(tlsPeerContext(cert, true), 1); err != nil {
			t.Fatalf("matching verified SAN rejected: %v", err)
		}
		if err := verify(tlsPeerContext(cert, false), 1); err == nil {
			t.Fatal("unverified certificate accepted")
		}
	})
}
