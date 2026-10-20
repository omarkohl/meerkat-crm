package httputil

import (
	"context"
	"errors"
	"io"
	"meerkat/netguard"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxFetches = 4

var (
	transport  http.RoundTripper = netguard.NewTransport(errors.New("failed to resolve hostname"), errors.New("access to internal IP addresses is not allowed"))
	fetchSlots                   = make(chan struct{}, maxFetches)
)

// validateURL accepts http and https URLs on the web ports. The address is checked by netguard when it is dialled.
func validateURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, errors.New("invalid URL format")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, errors.New("only http and https URLs are allowed")
	}

	if parsedURL.Hostname() == "" {
		return nil, errors.New("URL must have a host")
	}

	if port := parsedURL.Port(); port != "" && port != "80" && port != "443" {
		return nil, errors.New("only ports 80 and 443 are allowed")
	}

	return parsedURL, nil
}

// FetchImageFromURL fetches an image from a URL that a user supplied. The risk and the
// remaining exposure are in docs/adr/0017-fetching-user-supplied-urls.md.
func FetchImageFromURL(ctx context.Context, imageURL string) ([]byte, string, error) {
	// Clean URL - remove spaces and newlines (Google VCF format may have these)
	cleanURL := strings.ReplaceAll(imageURL, " ", "")
	cleanURL = strings.ReplaceAll(cleanURL, "\n", "")
	cleanURL = strings.ReplaceAll(cleanURL, "\r", "")

	parsedURL, err := validateURL(cleanURL)
	if err != nil {
		return nil, "", err
	}

	select {
	case fetchSlots <- struct{}{}:
		defer func() { <-fetchSlots }()
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if _, err := validateURL(req.URL.String()); err != nil {
				return errors.New("redirect to disallowed location")
			}
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, "", err
	}

	// Set a user agent to avoid being blocked by some servers
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MeerkatCRM/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("failed to fetch image: remote server returned " + resp.Status)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", errors.New("URL does not point to an image")
	}

	// Limit response size (10MB)
	const maxSize = 10 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, "", err
	}

	if len(body) > maxSize {
		return nil, "", errors.New("image is too large, maximum size is 10MB")
	}

	return body, contentType, nil
}
