// Package docuware is a small client for the DocuWare Platform REST API.
//
// It follows the hypermedia links DocuWare returns where they exist and falls
// back to the documented paths under /DocuWare/Platform otherwise.
package docuware

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PlatformPath is the path prefix of the DocuWare Platform REST API.
const PlatformPath = "/DocuWare/Platform"

const maxThrottleRetries = 3

// Cache stores raw JSON responses for metadata that rarely changes
// (file cabinets, dialogs). Implementations decide on expiry.
type Cache interface {
	Get(key string) ([]byte, bool)
	Put(key string, data []byte)
}

// Options configures a Client.
type Options struct {
	URL         string
	Credentials Credentials
	// Token is a previously issued token; it is reused while still valid.
	Token Token
	// OnToken is called whenever a new token is issued, so callers can persist it.
	OnToken    func(Token)
	Insecure   bool
	UserAgent  string
	Cache      Cache
	HTTPClient *http.Client
}

// Client talks to one DocuWare server.
type Client struct {
	root      *url.URL
	hc        *http.Client
	creds     Credentials
	token     Token
	onToken   func(Token)
	userAgent string
	cache     Cache
	now       func() time.Time
}

// New creates a client. It does not contact the server.
func New(o Options) (*Client, error) {
	root, err := NormalizeURL(o.URL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(root)
	if err != nil {
		return nil, err
	}
	hc := o.HTTPClient
	if hc == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.ResponseHeaderTimeout = 90 * time.Second
		if o.Insecure {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed on-prem servers
		}
		hc = &http.Client{Transport: tr}
	}
	ua := o.UserAgent
	if ua == "" {
		ua = "dw-cli"
	}
	return &Client{
		root:      u,
		hc:        hc,
		creds:     o.Credentials,
		token:     o.Token,
		onToken:   o.OnToken,
		userAgent: ua,
		cache:     o.Cache,
		now:       time.Now,
	}, nil
}

// BaseURL returns the server root, e.g. https://acme.docuware.cloud.
func (c *Client) BaseURL() string { return c.root.String() }

// NormalizeURL turns "acme", "acme.docuware.cloud", "https://host" or a full
// Platform URL into the server root (scheme://host[:port]). A bare name
// without dots is treated as a DocuWare Cloud tenant.
func NormalizeURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", errors.New("empty DocuWare URL")
	}
	if !strings.Contains(v, "://") {
		host := v
		if i := strings.IndexAny(host, "/:"); i >= 0 {
			host = host[:i]
		}
		if !strings.Contains(host, ".") && host != "localhost" {
			v = host + ".docuware.cloud" + v[len(host):]
		}
		v = "https://" + v
	}
	u, err := url.Parse(v)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid DocuWare URL %q", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

type request struct {
	method      string
	path        string
	query       url.Values
	body        []byte
	contentType string
	accept      string
}

// resolve turns a server-relative path or link into an absolute URL on the
// configured server. Links pointing at other hosts are refused so the bearer
// token never leaves the DocuWare server.
func (c *Client) resolve(path string, query url.Values) (string, error) {
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", path, err)
	}
	u := c.root.ResolveReference(ref)
	if !strings.EqualFold(u.Host, c.root.Host) {
		return "", fmt.Errorf("refusing to send credentials to %s (DocuWare server is %s)", u.Host, c.root.Host)
	}
	if len(query) > 0 {
		q := u.Query()
		for k, vs := range query {
			q[k] = vs
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (c *Client) do(ctx context.Context, r request) (*http.Response, error) {
	refreshed, force, throttled := false, false, 0
	for {
		resp, err := c.send(ctx, r, force)
		force = false
		if err != nil {
			return nil, err
		}
		switch {
		case resp.StatusCode == http.StatusUnauthorized && !refreshed && c.creds.complete():
			discard(resp)
			refreshed, force = true, true
			continue
		case resp.StatusCode == http.StatusTooManyRequests && throttled < maxThrottleRetries:
			wait := retryAfter(resp.Header.Get("Retry-After"), throttled)
			discard(resp)
			throttled++
			if err := sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode >= 400:
			apiErr := newAPIError(resp)
			resp.Body.Close()
			return nil, apiErr
		}
		return resp, nil
	}
}

func (c *Client) send(ctx context.Context, r request, forceToken bool) (*http.Response, error) {
	target, err := c.resolve(r.path, r.query)
	if err != nil {
		return nil, err
	}
	tok, err := c.accessToken(ctx, forceToken)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if r.body != nil {
		body = bytes.NewReader(r.body)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	accept := r.accept
	if accept == "" {
		accept = "application/json"
	}
	req.Header.Set("Accept", accept)
	if r.contentType != "" {
		req.Header.Set("Content-Type", r.contentType)
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", r.method, target, err)
	}
	return resp, nil
}

// Get fetches a path and returns the raw response body.
func (c *Client) Get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	resp, err := c.do(ctx, request{method: http.MethodGet, path: path, query: query})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	b, err := c.Get(ctx, path, query)
	if err != nil {
		return err
	}
	return decode(path, b, out)
}

// getCached is getJSON backed by the metadata cache.
func (c *Client) getCached(ctx context.Context, path string, out any) error {
	if c.cache != nil {
		if b, ok := c.cache.Get(path); ok && json.Unmarshal(b, out) == nil {
			return nil
		}
	}
	b, err := c.Get(ctx, path, nil)
	if err != nil {
		return err
	}
	if err := decode(path, b, out); err != nil {
		return err
	}
	if c.cache != nil {
		c.cache.Put(path, b)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, path string, query url.Values, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := c.do(ctx, request{
		method: http.MethodPost, path: path, query: query,
		body: body, contentType: "application/json",
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return decode(path, b, out)
}

func decode(path string, b []byte, out any) error {
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("unexpected response from %s: %w", path, err)
	}
	return nil
}

func discard(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
}

func retryAfter(header string, attempt int) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && s >= 0 {
		return min(time.Duration(s)*time.Second, 30*time.Second)
	}
	return time.Duration(1<<attempt) * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
