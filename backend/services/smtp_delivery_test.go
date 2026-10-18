package services

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"meerkat/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSMTP struct {
	host, port string
	password   string
	starttls   bool
	rejectRcpt bool

	mu       sync.Mutex
	messages []string
	rcpts    []string
}

func (s *fakeSMTP) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.rcpts...)
}

func (s *fakeSMTP) delivered() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.messages...)
}

// startFakeSMTP runs a one-connection-at-a-time SMTP server. With implicitTLS the
// connection is wrapped in TLS from the start; its certificate is not trusted by the client.
func startFakeSMTP(t *testing.T, s *fakeSMTP, implicitTLS bool) config.Config {
	t.Helper()
	cert := httptest.NewTLSServer(nil)
	t.Cleanup(cert.Close)
	tlsCfg := &tls.Config{Certificates: cert.TLS.Certificates, MinVersion: tls.VersionTLS12}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	if implicitTLS {
		ln = tls.NewListener(ln, tlsCfg)
	}
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	s.host, s.port = host, port

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.serve(conn, tlsCfg)
		}
	}()

	portNum, _ := strconv.Atoi(port)
	return config.Config{
		UseSMTP: true, SMTPHost: host, SMTPPort: portNum, SMTPFromEmail: "meerkat@example.com",
		SMTPUsername: "user", SMTPPassword: "secret", SMTPUseTLS: implicitTLS,
	}
}

func (s *fakeSMTP) serve(conn net.Conn, tlsCfg *tls.Config) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	say := func(line string) { _, _ = conn.Write([]byte(line + "\r\n")) }
	say("220 fake ready")
	var data strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "EHLO"):
			say("250-fake")
			if s.starttls {
				say("250-STARTTLS")
			}
			say("250 AUTH PLAIN")
		case line == "STARTTLS":
			say("220 go ahead")
			tc := tls.Server(conn, tlsCfg)
			if tc.Handshake() != nil {
				return
			}
			conn, r = tc, bufio.NewReader(tc)
			say = func(l string) { _, _ = tc.Write([]byte(l + "\r\n")) }
		case strings.HasPrefix(line, "AUTH PLAIN "):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 && parts[2] == s.password {
				say("235 ok")
			} else {
				say("535 bad credentials")
			}
		case strings.HasPrefix(line, "MAIL FROM"):
			say("250 ok")
		case strings.HasPrefix(line, "RCPT TO"):
			if s.rejectRcpt {
				say("550 no such user")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, line)
			s.mu.Unlock()
			say("250 ok")
		case line == "DATA":
			say("354 go on")
			data.Reset()
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				data.WriteString(l)
			}
			s.mu.Lock()
			s.messages = append(s.messages, data.String())
			s.mu.Unlock()
			say("250 queued")
		case line == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

var greeting = EmailMessage{To: "ada@example.com", Subject: "Birthday today", HTML: "<p>Ada turns 36</p>"}

// @006-US2
func TestSMTPDeliversTheMessageWithItsRecipientAndBody(t *testing.T) {
	srv := &fakeSMTP{password: "secret"}
	cfg := startFakeSMTP(t, srv, false)

	require.NoError(t, SendEmail(cfg, greeting))

	got := srv.delivered()
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "Subject: Birthday today")
	assert.Contains(t, got[0], "<p>Ada turns 36</p>")
	require.Len(t, srv.recipients(), 1)
	assert.Contains(t, srv.recipients()[0], "ada@example.com")
}

// @006-US2
func TestSMTPFailuresReachTheCallerAndNothingIsDelivered(t *testing.T) {
	cases := map[string]struct {
		srv      *fakeSMTP
		implicit bool
		want     string
	}{
		"wrong password":            {srv: &fakeSMTP{password: "other"}, want: "smtp auth"},
		"recipient refused":         {srv: &fakeSMTP{password: "secret", rejectRcpt: true}, want: "smtp rcpt to"},
		"untrusted TLS certificate": {srv: &fakeSMTP{password: "secret"}, implicit: true, want: "tls dial"},
		"untrusted STARTTLS":        {srv: &fakeSMTP{password: "secret", starttls: true}, want: "smtp starttls"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := startFakeSMTP(t, tc.srv, tc.implicit)

			err := SendEmail(cfg, greeting)

			require.Error(t, err)
			assert.ErrorContains(t, err, "all email channels failed")
			assert.ErrorContains(t, err, tc.want)
			assert.Empty(t, tc.srv.delivered())
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// @006-US2
func TestResendDeliversThroughTheAPIAndReportsItsRefusal(t *testing.T) {
	var gotAuth, gotBody string
	status := http.StatusOK
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		payload := `{"id":"mail-1"}`
		if status != http.StatusOK {
			payload = `{"name":"validation_error","message":"invalid from address","statusCode":422}`
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{"Content-Type": {"application/json"}}, Request: r}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = old })
	cfg := config.Config{UseResend: true, ResendAPIKey: "re_key", ResendFromEmail: "noreply@example.com"}

	require.NoError(t, SendEmail(cfg, greeting))
	assert.Equal(t, "Bearer re_key", gotAuth)
	assert.Contains(t, gotBody, "ada@example.com")
	assert.Contains(t, gotBody, "Birthday today")

	status = http.StatusUnprocessableEntity
	err := SendEmail(cfg, greeting)
	assert.ErrorContains(t, err, "all email channels failed")
	assert.ErrorContains(t, err, "resend")
}
