// Package config stores dw profiles, secrets, tokens and the metadata cache.
//
// Layout (overridable with DW_CONFIG_DIR / DW_CACHE_DIR):
//
//	<user config dir>/dw/config.json    profiles, no secrets
//	<user config dir>/dw/secrets.json   only when the OS keyring is unavailable
//	<user cache dir>/dw/<identity>/     token.json and cached metadata
//
// Passwords and client secrets go to the OS keyring (Windows Credential
// Manager, macOS Keychain, Secret Service) unless DW_SECRET_STORE=file.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/rechedev9/docuware-cli/internal/docuware"
)

const keyringService = "dw-cli"

// Profile is a saved connection. Secrets are stored separately.
type Profile struct {
	URL      string `json:"url"`
	Method   string `json:"method"`
	Username string `json:"username,omitempty"`
	ClientID string `json:"client_id,omitempty"`
	Insecure bool   `json:"insecure,omitempty"`
}

// File is the content of config.json.
type File struct {
	Current  string             `json:"current,omitempty"`
	Profiles map[string]Profile `json:"profiles"`
}

// Store gives access to the on-disk state.
type Store struct {
	ConfigDir   string
	CacheDir    string
	fileSecrets bool
}

// Open locates the config and cache directories.
func Open(getenv func(string) string) (*Store, error) {
	cfgDir := getenv("DW_CONFIG_DIR")
	if cfgDir == "" {
		d, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		cfgDir = filepath.Join(d, "dw")
	}
	cacheDir := getenv("DW_CACHE_DIR")
	if cacheDir == "" {
		d, err := os.UserCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = filepath.Join(d, "dw")
	}
	return &Store{ConfigDir: cfgDir, CacheDir: cacheDir, fileSecrets: getenv("DW_SECRET_STORE") == "file"}, nil
}

func (s *Store) configPath() string  { return filepath.Join(s.ConfigDir, "config.json") }
func (s *Store) secretsPath() string { return filepath.Join(s.ConfigDir, "secrets.json") }

// Load reads config.json; a missing file yields an empty config.
func (s *Store) Load() (File, error) {
	f := File{Profiles: map[string]Profile{}}
	b, err := os.ReadFile(s.configPath())
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("reading %s: %w", s.configPath(), err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	return f, nil
}

// Save writes config.json.
func (s *Store) Save(f File) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.configPath(), b)
}

// Secret returns the stored password or client secret of a profile.
func (s *Store) Secret(profile string) (string, error) {
	if !s.fileSecrets {
		if v, err := keyring.Get(keyringService, profile); err == nil {
			return v, nil
		}
	}
	secrets, err := s.readSecrets()
	if err != nil {
		return "", err
	}
	v, ok := secrets[profile]
	if !ok {
		return "", fmt.Errorf("no stored secret for profile %q", profile)
	}
	return v, nil
}

// SetSecret stores a secret, preferring the OS keyring. It reports whether
// it had to fall back to the plain secrets file.
func (s *Store) SetSecret(profile, secret string) (usedFile bool, err error) {
	if !s.fileSecrets {
		if err := keyring.Set(keyringService, profile, secret); err == nil {
			return false, s.removeFileSecret(profile)
		}
	}
	secrets, err := s.readSecrets()
	if err != nil {
		return true, err
	}
	secrets[profile] = secret
	b, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return true, err
	}
	return true, writeFileAtomic(s.secretsPath(), b)
}

// DeleteSecret removes a profile's secret from wherever it is stored.
func (s *Store) DeleteSecret(profile string) error {
	if !s.fileSecrets {
		if err := keyring.Delete(keyringService, profile); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			return err
		}
	}
	return s.removeFileSecret(profile)
}

func (s *Store) readSecrets() (map[string]string, error) {
	secrets := map[string]string{}
	b, err := os.ReadFile(s.secretsPath())
	if errors.Is(err, os.ErrNotExist) {
		return secrets, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &secrets); err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.secretsPath(), err)
	}
	return secrets, nil
}

func (s *Store) removeFileSecret(profile string) error {
	secrets, err := s.readSecrets()
	if err != nil || len(secrets) == 0 {
		return err
	}
	if _, ok := secrets[profile]; !ok {
		return nil
	}
	delete(secrets, profile)
	b, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.secretsPath(), b)
}

// IdentityKey names the cache directory of one server + account pair, so
// tokens and metadata never leak between accounts.
func IdentityKey(url, method, account string) string {
	h := sha256.Sum256([]byte(url + "\x00" + method + "\x00" + account))
	return hex.EncodeToString(h[:8])
}

func (s *Store) identityDir(key string) string { return filepath.Join(s.CacheDir, key) }

// LoadToken returns the cached token, or a zero token.
func (s *Store) LoadToken(key string) docuware.Token {
	var t docuware.Token
	b, err := os.ReadFile(filepath.Join(s.identityDir(key), "token.json"))
	if err == nil {
		_ = json.Unmarshal(b, &t)
	}
	return t
}

// SaveToken caches a token.
func (s *Store) SaveToken(key string, t docuware.Token) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(s.identityDir(key), "token.json"), b)
}

// ClearIdentity deletes the cached token and metadata of an identity.
func (s *Store) ClearIdentity(key string) error {
	return os.RemoveAll(s.identityDir(key))
}

// Cache returns the metadata cache of an identity.
func (s *Store) Cache(key string, ttl time.Duration) docuware.Cache {
	return fileCache{dir: filepath.Join(s.identityDir(key), "meta"), ttl: ttl}
}

type fileCache struct {
	dir string
	ttl time.Duration
}

func (c fileCache) path(key string) string {
	h := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, hex.EncodeToString(h[:12])+".json")
}

func (c fileCache) Get(key string) ([]byte, bool) {
	p := c.path(key)
	fi, err := os.Stat(p)
	if err != nil || time.Since(fi.ModTime()) > c.ttl {
		return nil, false
	}
	b, err := os.ReadFile(p)
	return b, err == nil
}

func (c fileCache) Put(key string, data []byte) {
	_ = writeFileAtomic(c.path(key), data)
}

// writeFileAtomic writes via a temp file and rename, with owner-only permissions.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
