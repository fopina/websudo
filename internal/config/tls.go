package config

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// EnsureTLSAssets makes sure CA files exist for TLS-capable proxying.
func EnsureTLSAssets(_ context.Context, cfg *Config) error {
	certExists, keyExists := fileExists(cfg.TLS.CAcertPath), fileExists(cfg.TLS.CAkeyPath)
	if certExists && keyExists {
		return nil
	}
	if !cfg.TLS.GenerateOnBoot {
		return fmt.Errorf("CA files are missing: cert=%s key=%s", cfg.TLS.CAcertPath, cfg.TLS.CAkeyPath)
	}

	return GenerateCA(cfg.TLS.CAcertPath, cfg.TLS.CAkeyPath)
}

// GenerateCA creates a self-signed CA certificate and key.
func GenerateCA(certPath, keyPath string) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0o700); err != nil {
		return fmt.Errorf("create cert dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return fmt.Errorf("create key dir: %w", err)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate rsa key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "websudo Local CA",
			Organization: []string{"websudo"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}

	keyDER := x509.MarshalPKCS1PrivateKey(priv)
	if err := writePEM(keyPath, "RSA PRIVATE KEY", keyDER, 0o600); err != nil {
		return err
	}

	return nil
}

func writePEM(path string, blockType string, bytes []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: bytes}); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
