package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func generateCert(tmp string) (string, string, string, error) {
	now := time.Now()
	validityPeriod := time.Hour

	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now,
		NotAfter:              now.Add(validityPeriod),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate private key: %w", err)
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, caPub, caPriv)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create certificate: %w", err)
	}
	caPEM, err := os.Create(filepath.Join(tmp, "ca.crt"))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create ca.crt: %w", err)
	}
	defer caPEM.Close()
	if err := pem.Encode(caPEM, &pem.Block{Type: "CERTIFICATE", Bytes: caBytes}); err != nil {
		return "", "", "", fmt.Errorf("failed to encode PEM: %w", err)
	}

	cert := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             now,
		NotAfter:              now.Add(validityPeriod),
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
	}
	certPub, certPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate private key: %w", err)
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, certPub, caPriv)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create certificate: %w", err)
	}
	certPEM, err := os.Create(filepath.Join(tmp, "server.crt"))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create server.crt: %w", err)
	}
	if err := pem.Encode(certPEM, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes}); err != nil {
		return "", "", "", fmt.Errorf("failed to encode PEM: %w", err)
	}
	certKeyPEM, err := os.OpenFile(filepath.Join(tmp, "server.key"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create server.key: %w", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(certPriv)
	if err != nil {
		return "", "", "", fmt.Errorf("unable to marshal private key: %w", err)
	}
	if err := pem.Encode(certKeyPEM, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return "", "", "", fmt.Errorf("failed to encode PEM: %w", err)
	}

	return caPEM.Name(), certPEM.Name(), certKeyPEM.Name(), nil
}
