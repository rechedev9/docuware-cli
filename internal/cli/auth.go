package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rechedev9/docuware-cli/internal/config"
	"github.com/rechedev9/docuware-cli/internal/docuware"
)

func (a *app) loginCmd() *cobra.Command {
	var serverURL, user, clientID string
	var fromStdin, insecure bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to DocuWare and save the connection as a profile",
		Long: `Sign in with a DocuWare user (OAuth2 password grant) or a registered
application (--client-id, OAuth2 client credentials grant).

The password or client secret is prompted without echo, or read from stdin
with --password-stdin. It is stored in the OS keyring (Windows Credential
Manager, macOS Keychain, Secret Service); access tokens are cached and renewed
automatically.

--url accepts a DocuWare Cloud tenant name ("acme"), a host name or a full URL.
For on-premises servers without dots in the name, pass the scheme
("https://dwserver").`,
		Example: `  dw login --url acme --user peggy.jenkins
  dw login --url https://dms.example.com --user svc-reader --password-stdin < pw.txt
  dw login --url acme --client-id 1b2c... --password-stdin -p service`,
		Args: exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if serverURL == "" {
				return usageErr("--url is required (tenant name, host or full URL)")
			}
			if (user == "") == (clientID == "") {
				return usageErr("pass either --user or --client-id")
			}
			root, err := docuware.NormalizeURL(serverURL)
			if err != nil {
				return usageErr("%v", err)
			}
			creds := docuware.Credentials{Method: docuware.MethodPassword, Username: user}
			account, label, envVar := user, "password", "DW_PASSWORD"
			if clientID != "" {
				creds = docuware.Credentials{Method: docuware.MethodClientCredentials, ClientID: clientID}
				account, label, envVar = clientID, "client secret", "DW_CLIENT_SECRET"
			}
			secret, err := a.readSecret(label, envVar, fromStdin)
			if err != nil {
				return err
			}
			if creds.Method == docuware.MethodPassword {
				creds.Password = secret
			} else {
				creds.ClientSecret = secret
			}

			c, err := docuware.New(docuware.Options{URL: root, Credentials: creds, Insecure: insecure, UserAgent: "dw-cli/" + a.version})
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			tok, err := c.Login(ctx)
			if err != nil {
				return err
			}
			version, err := c.Version(ctx)
			if err != nil {
				return err
			}

			store, err := a.openStore()
			if err != nil {
				return err
			}
			f, err := store.Load()
			if err != nil {
				return err
			}
			name := a.profileName(f)
			f.Profiles[name] = config.Profile{URL: root, Method: creds.Method, Username: user, ClientID: clientID, Insecure: insecure}
			f.Current = name
			if err := store.Save(f); err != nil {
				return err
			}
			usedFile, err := store.SetSecret(name, secret)
			if err != nil {
				return fmt.Errorf("saving the %s: %w", label, err)
			}
			key := config.IdentityKey(root, creds.Method, strings.ToLower(account))
			_ = store.ClearIdentity(key)
			_ = store.SaveToken(key, tok)

			if a.jsonOut {
				return a.printJSON(map[string]any{
					"profile": name, "url": root, "method": creds.Method, "account": account,
					"version": version, "secret_store": secretStoreName(usedFile),
				})
			}
			fprintf(a.stdout, "Logged in to %s (DocuWare %s) as %s, profile %q.\n", root, version, account, name)
			if usedFile {
				fprintf(a.stderr, "warning: OS keyring unavailable; the %s is stored in %s\n", label, store.ConfigDir)
			}
			return nil
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&serverURL, "url", "", "DocuWare server: tenant name, host or URL")
	fl.StringVarP(&user, "user", "u", "", "DocuWare user name (password grant)")
	fl.StringVar(&clientID, "client-id", "", "application client id (client credentials grant)")
	fl.BoolVar(&fromStdin, "password-stdin", false, "read the password or client secret from stdin")
	fl.BoolVar(&insecure, "insecure", false, "skip TLS verification (self-signed on-premises servers only)")
	return cmd
}

func secretStoreName(usedFile bool) string {
	if usedFile {
		return "file"
	}
	return "keyring"
}

func (a *app) readSecret(label, envVar string, fromStdin bool) (string, error) {
	if fromStdin {
		b, err := io.ReadAll(io.LimitReader(a.stdin, 64<<10))
		if err != nil {
			return "", err
		}
		s := strings.TrimRight(string(b), "\r\n")
		if s == "" {
			return "", usageErr("empty %s on stdin", label)
		}
		return s, nil
	}
	if v := a.getenv(envVar); v != "" {
		return v, nil
	}
	if f, ok := a.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		fprintf(a.stderr, "DocuWare %s: ", label)
		b, err := term.ReadPassword(int(f.Fd()))
		fprintf(a.stderr, "\n")
		if err != nil {
			return "", err
		}
		if len(b) == 0 {
			return "", usageErr("empty %s", label)
		}
		return string(b), nil
	}
	return "", usageErr("no terminal to prompt for the %s; use --password-stdin or set %s", label, envVar)
}

func (a *app) logoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the profile, its stored secret and cached tokens",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := a.openStore()
			if err != nil {
				return err
			}
			f, err := store.Load()
			if err != nil {
				return err
			}
			name := a.profileName(f)
			p, ok := f.Profiles[name]
			if !ok {
				return &docuware.NotFoundError{Kind: "profile", Key: name, Available: profileNames(f)}
			}
			account := p.Username
			if p.Method == docuware.MethodClientCredentials {
				account = p.ClientID
			}
			_ = store.ClearIdentity(config.IdentityKey(p.URL, p.Method, strings.ToLower(account)))
			if err := store.DeleteSecret(name); err != nil {
				return err
			}
			delete(f.Profiles, name)
			if f.Current == name {
				f.Current = ""
			}
			if err := store.Save(f); err != nil {
				return err
			}
			if a.jsonOut {
				return a.printJSON(map[string]any{"profile": name, "logged_out": true})
			}
			fprintf(a.stdout, "Logged out of %s (profile %q).\n", p.URL, name)
			return nil
		},
	}
}

func profileNames(f config.File) []string {
	names := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		names = append(names, n)
	}
	return names
}

func (a *app) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check the connection and show what this account can see",
		Args:  exactArgs(0, "no arguments"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := a.connect(ctx)
			if err != nil {
				return err
			}
			version, err := c.Version(ctx)
			if err != nil {
				return err
			}
			orgs, err := c.Organizations(ctx)
			if err != nil {
				return err
			}
			cabinets, err := c.FileCabinets(ctx)
			if err != nil {
				return err
			}
			var orgNames []string
			for _, o := range orgs {
				orgNames = append(orgNames, o.Name)
			}
			nCab, nBasket := 0, 0
			for _, fc := range cabinets {
				if fc.IsBasket {
					nBasket++
				} else {
					nCab++
				}
			}
			expiry := c.Token().Expiry
			if a.jsonOut {
				return a.printJSON(map[string]any{
					"url": a.sess.url, "profile": a.sess.profile, "method": a.sess.creds.Method,
					"account": a.sess.account(), "credentials_from_env": a.sess.fromEnv,
					"version": version, "organizations": orgNames,
					"file_cabinets": nCab, "baskets": nBasket,
					"token_expires": expiry.Format(time.RFC3339),
				})
			}
			source := "profile " + fmt.Sprintf("%q", a.sess.profile)
			if a.sess.fromEnv {
				source = "environment"
			}
			fprintf(a.stdout, "Server:    %s (DocuWare %s)\n", a.sess.url, version)
			fprintf(a.stdout, "Account:   %s (%s, from %s)\n", a.sess.account(), a.sess.creds.Method, source)
			fprintf(a.stdout, "Org:       %s\n", strings.Join(orgNames, ", "))
			fprintf(a.stdout, "Cabinets:  %s, %s\n", plural(nCab, "file cabinet"), plural(nBasket, "document tray"))
			fprintf(a.stdout, "Token:     valid until %s\n", expiry.Local().Format("2006-01-02 15:04"))
			return nil
		},
	}
}
