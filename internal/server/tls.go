package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/fopina/websudo/internal/config"
)

func applyTLSConfig(proxy *goproxy.ProxyHttpServer, cfg *config.Config) error {
	ca, err := loadCA(cfg.TLS.CAcertPath, cfg.TLS.CAkeyPath)
	if err != nil {
		return err
	}

	mitmAction := &goproxy.ConnectAction{
		Action:    goproxy.ConnectMitm,
		TLSConfig: goproxy.TLSConfigFromCA(ca),
	}

	proxy.OnRequest().HandleConnectFunc(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
		if shouldInterceptTLS(cfg, host) {
			return mitmAction, host
		}
		if cfg.AllowUnconfiguredDestinations {
			return goproxy.OkConnect, host
		}
		return goproxy.RejectConnect, host
	})

	return nil
}

func shouldInterceptTLS(cfg *config.Config, host string) bool {
	host = stripPort(host)
	for _, svc := range cfg.Services {
		if svc.MatchHost != "" && strings.EqualFold(host, svc.MatchHost) {
			return true
		}
	}
	return false
}

func stripPort(host string) string {
	if idx := strings.Index(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}

func loadCA(certPath, keyPath string) (*tls.Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %q: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key %q: %w", keyPath, err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load CA keypair: %w", err)
	}
	if len(cert.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse CA certificate: %w", err)
		}
		cert.Leaf = leaf
	}

	return &cert, nil
}
