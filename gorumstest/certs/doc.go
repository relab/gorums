// Package certs builds throwaway X.509 certificates for tests.
// It depends only on the standard library, so any test package can import it.
//
// [New] signs a certificate from an explicit template, so a test can express
// any SAN set, including missing or invalid SANs.
// [Localhost], [Loopback], and [NewCA] cover the common cases.
package certs
