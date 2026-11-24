package httptransport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeClientPair writes a self-signed client certificate and its key for
// commonName, and stamps both files with modTime so that two successive
// versions never share a modification time.
func writeClientPair(t *testing.T, certPath, keyPath, commonName string, modTime time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(modTime.UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	writeStamped(t, certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), modTime)
	writeStamped(t, keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), modTime)
}

func writeStamped(t *testing.T, path string, content []byte, modTime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

// newMTLSServer starts a TLS server that requires a client certificate and
// answers with the common name of the one presented.
func newMTLSServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.TLS.PeerCertificates[0].Subject.CommonName))
	}))
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAnyClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// presentedCN makes one request on a fresh connection and returns the
// common name the server saw.
func presentedCN(t *testing.T, c *Client, serverCert *x509.Certificate) string {
	t.Helper()
	transport, ok := c.hc.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T, want *http.Transport", c.hc.Transport)
	}
	if transport.TLSClientConfig.RootCAs == nil {
		pool := x509.NewCertPool()
		pool.AddCert(serverCert)
		transport.TLSClientConfig.RootCAs = pool
	}
	// Drop the pooled connection: the certificate is chosen per handshake.
	transport.CloseIdleConnections()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, c.baseURL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 128)
	n, _ := resp.Body.Read(buf)
	return string(buf[:n])
}

// certTestBase is the modification time of the first pair; each renewal is
// stamped a minute later.
func certTestBase() time.Time { return time.Now().Add(-time.Hour).Truncate(time.Second) }

// newReloadFixture writes a first pair ("collector-v1") and builds a Client
// presenting it to a server that requires a client certificate.
func newReloadFixture(t *testing.T, base time.Time) (c *Client, srv *httptest.Server, certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	certPath, keyPath = filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writeClientPair(t, certPath, keyPath, "collector-v1", base)
	srv = newMTLSServer(t)
	c, err := New(&Config{ServerURL: srv.URL, ClientCert: certPath, ClientKey: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	return c, srv, certPath, keyPath
}

func TestClientCertificateRenewalIsPresentedOnNextConnection(t *testing.T) {
	t.Parallel()
	base := certTestBase()
	c, srv, certPath, keyPath := newReloadFixture(t, base)
	if got := presentedCN(t, c, srv.Certificate()); got != "collector-v1" {
		t.Fatalf("first connection presented %q, want collector-v1", got)
	}

	writeClientPair(t, certPath, keyPath, "collector-v2", base.Add(time.Minute))

	if got := presentedCN(t, c, srv.Certificate()); got != "collector-v2" {
		t.Errorf("after renewal presented %q, want collector-v2", got)
	}
}

func TestClientCertificateBrokenRenewalKeepsPrevious(t *testing.T) {
	t.Parallel()
	base := certTestBase()
	c, srv, certPath, keyPath := newReloadFixture(t, base)

	writeStamped(t, certPath, []byte("half-written"), base.Add(time.Minute))

	if got := presentedCN(t, c, srv.Certificate()); got != "collector-v1" {
		t.Errorf("with a broken file presented %q, want collector-v1", got)
	}

	writeClientPair(t, certPath, keyPath, "collector-v3", base.Add(2*time.Minute))

	if got := presentedCN(t, c, srv.Certificate()); got != "collector-v3" {
		t.Errorf("after the file is fixed presented %q, want collector-v3", got)
	}
}

func TestClientCertificateUnchangedPairIsNotReloaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writeClientPair(t, certPath, keyPath, "collector-v1", certTestBase())
	source, err := newClientCertSource(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := source.getClientCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := source.getClientCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}

	if first != second {
		t.Error("the certificate was reloaded although the files did not change")
	}
}
