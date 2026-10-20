package httputil

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Photo URLs come from users and vCards: internal targets are always refused, whatever
// the BLOCK_PRIVATE_URLS setting.
func TestFetchImageFromURLRefusesInternalTargets(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
	}))
	defer srv.Close()
	for _, u := range []string{srv.URL + "/a.png", "http://localhost/a.png", "http://0.0.0.0/a.png", "http://[::ffff:127.0.0.1]/a.png", "http://169.254.169.254/latest/meta-data", "http://100.100.100.200/"} {
		_, _, err := FetchImageFromURL(context.Background(), u)
		require.Error(t, err, u)
	}
	assert.Zero(t, hits.Load())
}

// routeTo sends every connection to srv, whatever host the URL names, and counts the dials.
func routeTo(t *testing.T, srv *httptest.Server) *atomic.Int32 {
	var dials atomic.Int32
	old := transport
	t.Cleanup(func() { transport = old })
	transport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}}
	return &dials
}

func imageServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func servePNG(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write([]byte("png"))
}

// @004-US9
func TestFetchImageFromURLFetchesFromTheWebPorts(t *testing.T) {
	routeTo(t, imageServer(t, servePNG))
	for _, u := range []string{"http://images.example/a.png", "http://images.example:80/a.png", "http://images.example:443/a.png"} {
		body, contentType, err := FetchImageFromURL(context.Background(), u)
		require.NoError(t, err, u)
		assert.Equal(t, "png", string(body))
		assert.Equal(t, "image/png", contentType)
	}
}

// @004-US9
func TestFetchImageFromURLRefusesOtherPorts(t *testing.T) {
	dials := routeTo(t, imageServer(t, servePNG))
	for _, u := range []string{"http://images.example:8080/a.png", "https://images.example:22/a.png", "http://images.example:0/a.png"} {
		_, _, err := FetchImageFromURL(context.Background(), u)
		require.Error(t, err, u)
	}
	assert.Zero(t, dials.Load())
}

// @004-US9
func TestFetchImageFromURLRefusesRedirectsToOtherPorts(t *testing.T) {
	routeTo(t, imageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/a.png" {
			http.Redirect(w, r, "http://images.example:6379/b.png", http.StatusFound)
			return
		}
		servePNG(w, r)
	}))
	_, _, err := FetchImageFromURL(context.Background(), "http://images.example/a.png")
	require.Error(t, err)
}

// @004-US9
func TestFetchImageFromURLWaitsWhileTheServerFetchesFourImages(t *testing.T) {
	release := make(chan struct{})
	var started atomic.Int32
	routeTo(t, imageServer(t, func(w http.ResponseWriter, r *http.Request) {
		started.Add(1)
		<-release
		servePNG(w, r)
	}))
	done := make(chan error, maxFetches)
	for range maxFetches {
		go func() {
			_, _, err := FetchImageFromURL(context.Background(), "http://images.example/a.png")
			done <- err
		}()
	}
	require.Eventually(t, func() bool { return started.Load() == maxFetches }, 5*time.Second, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := FetchImageFromURL(ctx, "http://images.example/a.png")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, int32(maxFetches), started.Load())

	close(release)
	for range maxFetches {
		require.NoError(t, <-done)
	}
}
