package docuware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Supported OAuth2 grants.
const (
	MethodPassword          = "password"
	MethodClientCredentials = "client_credentials"
)

const (
	// passwordClientID is DocuWare's built-in public client for the password grant.
	passwordClientID = "docuware.platform.net.client"
	platformScope    = "docuware.platform"
)

// Credentials identify a DocuWare user (password grant) or a registered
// application (client credentials grant).
type Credentials struct {
	Method       string
	Username     string
	Password     string
	ClientID     string
	ClientSecret string
}

func (c Credentials) complete() bool {
	switch c.Method {
	case MethodPassword:
		return c.Username != "" && c.Password != ""
	case MethodClientCredentials:
		return c.ClientID != "" && c.ClientSecret != ""
	}
	return false
}

// Token is an access token issued by the DocuWare Identity Service.
type Token struct {
	AccessToken string    `json:"access_token"`
	Expiry      time.Time `json:"expiry"`
	// TokenEndpoint is remembered to skip the two discovery requests next time.
	TokenEndpoint string `json:"token_endpoint,omitempty"`
}

// ValidAt reports whether the token can still be used at t, with a minute of slack.
func (t Token) ValidAt(now time.Time) bool {
	return t.AccessToken != "" && now.Add(time.Minute).Before(t.Expiry)
}

// Token returns the current token (possibly empty).
func (c *Client) Token() Token { return c.token }

// Login requests a fresh token, which validates the credentials.
func (c *Client) Login(ctx context.Context) (Token, error) {
	if _, err := c.accessToken(ctx, true); err != nil {
		return Token{}, err
	}
	return c.token, nil
}

func (c *Client) accessToken(ctx context.Context, force bool) (string, error) {
	if !force && c.token.ValidAt(c.now()) {
		return c.token.AccessToken, nil
	}
	if !c.creds.complete() {
		if c.token.AccessToken != "" {
			return "", fmt.Errorf("access token expired and no stored credentials: %w", ErrNoCredentials)
		}
		return "", ErrNoCredentials
	}
	tok, err := c.requestToken(ctx)
	if err != nil {
		return "", err
	}
	c.token = tok
	if c.onToken != nil {
		c.onToken(tok)
	}
	return tok.AccessToken, nil
}

func (c *Client) requestToken(ctx context.Context) (Token, error) {
	form := url.Values{}
	switch c.creds.Method {
	case MethodPassword:
		form.Set("grant_type", "password")
		form.Set("username", c.creds.Username)
		form.Set("password", c.creds.Password)
		form.Set("client_id", passwordClientID)
		form.Set("scope", platformScope)
	case MethodClientCredentials:
		form.Set("grant_type", "client_credentials")
		form.Set("client_id", c.creds.ClientID)
		form.Set("client_secret", c.creds.ClientSecret)
		form.Set("scope", platformScope)
	default:
		return Token{}, fmt.Errorf("unsupported auth method %q", c.creds.Method)
	}

	endpoint, cached := c.token.TokenEndpoint, true
	if endpoint == "" {
		var err error
		if endpoint, err = c.discoverTokenEndpoint(ctx); err != nil {
			return Token{}, err
		}
		cached = false
	}
	tok, status, err := c.postToken(ctx, endpoint, form)
	if cached && (status == http.StatusNotFound || status == 0) {
		// The remembered endpoint moved; discover it again once.
		if endpoint, err = c.discoverTokenEndpoint(ctx); err != nil {
			return Token{}, err
		}
		tok, _, err = c.postToken(ctx, endpoint, form)
	}
	return tok, err
}

func (c *Client) postToken(ctx context.Context, endpoint string, form url.Values) (Token, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return Token{}, 0, fmt.Errorf("requesting token from %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var oe struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &oe)
		if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized {
			return Token{}, resp.StatusCode, &AuthError{Reason: firstNonEmpty(oe.Description, oe.Error, resp.Status)}
		}
		return Token{}, resp.StatusCode, fmt.Errorf("token endpoint %s: HTTP %d", endpoint, resp.StatusCode)
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil || tr.AccessToken == "" {
		return Token{}, resp.StatusCode, errors.New("token endpoint returned no access_token")
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	return Token{
		AccessToken:   tr.AccessToken,
		Expiry:        c.now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		TokenEndpoint: endpoint,
	}, resp.StatusCode, nil
}

// discoverTokenEndpoint follows DocuWare's discovery chain (KBA-37505):
// IdentityServiceInfo -> OpenID configuration -> token_endpoint.
func (c *Client) discoverTokenEndpoint(ctx context.Context) (string, error) {
	var info struct {
		IdentityServiceURL string `json:"IdentityServiceUrl"`
	}
	infoURL := c.root.String() + PlatformPath + "/Home/IdentityServiceInfo"
	if err := c.getPublic(ctx, infoURL, &info); err != nil {
		return "", fmt.Errorf("cannot discover the DocuWare Identity Service (OAuth2 needs DocuWare 7.10+): %w", err)
	}
	if info.IdentityServiceURL == "" {
		return "", errors.New("DocuWare returned no IdentityServiceUrl (OAuth2 needs DocuWare 7.10+)")
	}
	var oidc struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	discovery := strings.TrimRight(info.IdentityServiceURL, "/") + "/.well-known/openid-configuration"
	if err := c.getPublic(ctx, discovery, &oidc); err != nil {
		return "", fmt.Errorf("OpenID discovery failed: %w", err)
	}
	if oidc.TokenEndpoint == "" {
		return "", errors.New("OpenID configuration has no token_endpoint")
	}
	return oidc.TokenEndpoint, nil
}

func (c *Client) getPublic(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return newAPIError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
