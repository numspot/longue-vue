package httptransport

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// fileStamp identifies a version of a file on disk. A Kubernetes secret
// volume swaps a symlink on rotation, so the size and modification time of
// the resolved file change even though the path does not.
type fileStamp struct {
	modTime time.Time
	size    int64
}

// clientCertSource serves the mTLS client certificate of a collector and
// reloads it when the certificate or key file changes on disk.
//
// A collector runs for months while its certificate is renewed every few
// weeks: loading the pair once at start-up would end with an expired
// certificate. The files are checked on each TLS handshake, which is rare
// (connections are reused within a polling cycle), so no watcher goroutine
// is needed.
//
// If the new pair cannot be loaded (files half-written, key mismatch), the
// previous certificate keeps being served and the next handshake retries.
type clientCertSource struct {
	certPath string
	keyPath  string

	mu        sync.Mutex
	cert      *tls.Certificate
	certStamp fileStamp
	keyStamp  fileStamp
}

// newClientCertSource loads the pair once, so a broken configuration fails
// at start-up rather than on the first request.
func newClientCertSource(certPath, keyPath string) (*clientCertSource, error) {
	s := &clientCertSource{certPath: certPath, keyPath: keyPath}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// getClientCertificate satisfies tls.Config.GetClientCertificate.
func (s *clientCertSource) getClientCertificate(_ *tls.CertificateRequestInfo) (*tls.Certificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.changed() {
		if err := s.load(); err != nil {
			slog.Warn("client certificate reload failed; previous certificate retained",
				slog.String("path", s.certPath), slog.Any("error", err))
		} else {
			slog.Info("client certificate reloaded", slog.String("path", s.certPath))
		}
	}
	return s.cert, nil
}

// changed reports whether either file differs from the loaded version. A
// file that cannot be read is not a change: the loaded pair stays in use.
func (s *clientCertSource) changed() bool {
	certStamp, err := stampOf(s.certPath)
	if err != nil {
		return false
	}
	keyStamp, err := stampOf(s.keyPath)
	if err != nil {
		return false
	}
	return certStamp != s.certStamp || keyStamp != s.keyStamp
}

// load reads the pair and records the version of both files. On error the
// state is left untouched.
func (s *clientCertSource) load() error {
	certStamp, err := stampOf(s.certPath)
	if err != nil {
		return fmt.Errorf("load client cert/key: %w", err)
	}
	keyStamp, err := stampOf(s.keyPath)
	if err != nil {
		return fmt.Errorf("load client cert/key: %w", err)
	}
	cert, err := tls.LoadX509KeyPair(s.certPath, s.keyPath)
	if err != nil {
		return fmt.Errorf("load client cert/key: %w", err)
	}
	s.cert = &cert
	s.certStamp = certStamp
	s.keyStamp = keyStamp
	return nil
}

func stampOf(path string) (fileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, fmt.Errorf("stat %s: %w", path, err)
	}
	return fileStamp{modTime: info.ModTime(), size: info.Size()}, nil
}
