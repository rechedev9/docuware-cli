package cli

import (
	"context"
	"strings"

	"github.com/rechedev9/docuware-cli/internal/config"
	"github.com/rechedev9/docuware-cli/internal/docuware"
)

// session is the resolved connection: which server, as whom.
type session struct {
	profile  string
	url      string
	creds    docuware.Credentials
	insecure bool
	fromEnv  bool
}

func (s session) account() string {
	if s.creds.Method == docuware.MethodClientCredentials {
		return s.creds.ClientID
	}
	return s.creds.Username
}

func (s session) identityKey() string {
	return config.IdentityKey(s.url, s.creds.Method, strings.ToLower(s.account()))
}

func (a *app) openStore() (*config.Store, error) {
	if a.store == nil {
		s, err := config.Open(a.getenv)
		if err != nil {
			return nil, err
		}
		a.store = s
	}
	return a.store, nil
}

func (a *app) profileName(f config.File) string {
	switch {
	case a.profile != "":
		return a.profile
	case a.getenv("DW_PROFILE") != "":
		return a.getenv("DW_PROFILE")
	case f.Current != "":
		return f.Current
	}
	return "default"
}

// resolveSession combines the profile with DW_* environment overrides.
// Environment credentials win, which suits CI jobs and one-off agent runs.
func (a *app) resolveSession() (session, error) {
	store, err := a.openStore()
	if err != nil {
		return session{}, err
	}
	f, err := store.Load()
	if err != nil {
		return session{}, err
	}
	name := a.profileName(f)
	p := f.Profiles[name]
	s := session{profile: name, url: p.URL, insecure: p.Insecure}
	if v := a.getenv("DW_URL"); v != "" {
		s.url = v
	}
	if truthy(a.getenv("DW_INSECURE")) {
		s.insecure = true
	}
	switch {
	case a.getenv("DW_CLIENT_ID") != "" && a.getenv("DW_CLIENT_SECRET") != "":
		s.creds = docuware.Credentials{Method: docuware.MethodClientCredentials, ClientID: a.getenv("DW_CLIENT_ID"), ClientSecret: a.getenv("DW_CLIENT_SECRET")}
		s.fromEnv = true
	case a.getenv("DW_USERNAME") != "" && a.getenv("DW_PASSWORD") != "":
		s.creds = docuware.Credentials{Method: docuware.MethodPassword, Username: a.getenv("DW_USERNAME"), Password: a.getenv("DW_PASSWORD")}
		s.fromEnv = true
	default:
		s.creds = docuware.Credentials{Method: p.Method, Username: p.Username, ClientID: p.ClientID}
		if secret, err := store.Secret(name); err == nil {
			if p.Method == docuware.MethodClientCredentials {
				s.creds.ClientSecret = secret
			} else {
				s.creds.Password = secret
			}
		}
	}
	if s.url == "" {
		return s, docuware.ErrNoCredentials
	}
	root, err := docuware.NormalizeURL(s.url)
	if err != nil {
		return s, err
	}
	s.url = root
	return s, nil
}

// connect builds the client for the current session, reusing the cached token.
func (a *app) connect(_ context.Context) (*docuware.Client, error) {
	if a.client != nil {
		return a.client, nil
	}
	s, err := a.resolveSession()
	if err != nil {
		return nil, err
	}
	key := s.identityKey()
	opts := docuware.Options{
		URL:         s.url,
		Credentials: s.creds,
		Token:       a.store.LoadToken(key),
		OnToken:     func(t docuware.Token) { _ = a.store.SaveToken(key, t) },
		Insecure:    s.insecure,
		UserAgent:   "dw-cli/" + a.version,
	}
	if !a.noCache && !truthy(a.getenv("DW_NO_CACHE")) {
		opts.Cache = a.store.Cache(key, metadataTTL)
	}
	c, err := docuware.New(opts)
	if err != nil {
		return nil, err
	}
	a.client, a.sess = c, s
	return c, nil
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
