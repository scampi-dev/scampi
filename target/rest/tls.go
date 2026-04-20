// SPDX-License-Identifier: GPL-3.0-only

package rest

import (
	"crypto/tls"
	"crypto/x509"
)

// TLSConfig configures TLS for the REST target's HTTP transport.
// Implementations are constructed in scampi (rest.tls.secure,
// rest.tls.insecure, rest.tls.ca_cert) and stored in Config.TLS.
type TLSConfig interface {
	TLSClientConfig() *tls.Config
	Kind() string
}

// SecureTLSConfig uses the system CA pool. This is the default.
type SecureTLSConfig struct{}

func (SecureTLSConfig) Kind() string                 { return "secure" }
func (SecureTLSConfig) TLSClientConfig() *tls.Config { return nil }

// InsecureTLSConfig skips all certificate verification.
type InsecureTLSConfig struct{}

func (InsecureTLSConfig) Kind() string { return "insecure" }
func (InsecureTLSConfig) TLSClientConfig() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}

// CACertTLSConfig validates against a custom CA certificate.
type CACertTLSConfig struct {
	Pool *x509.CertPool
}

func (CACertTLSConfig) Kind() string { return "ca_cert" }
func (c CACertTLSConfig) TLSClientConfig() *tls.Config {
	return &tls.Config{RootCAs: c.Pool}
}
