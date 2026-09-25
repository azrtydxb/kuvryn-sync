package console

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/azrtydxb/kuvryn-sync/internal/redact"
)

// Cookie names. ksync_state carries the OAuth state and PKCE verifier, and
// ksync_nonce the ID token nonce, for the ten minutes a sign-in may take.
const (
	SessionCookie = "ksync_session"
	StateCookie   = "ksync_state"
	NonceCookie   = "ksync_nonce"

	loginWindow        = 10 * time.Minute
	maxSessionCookie   = 4000
	discoveryRetry     = 10 * time.Second
	systemPrefix       = "system:"
	callbackTimeout    = 15 * time.Second
	errTooManyGroups   = "too many groups for a session"
	emailClaim         = "email"
	emailVerifiedClaim = "email_verified"
)

// ErrNoSession reports a request without a valid, unexpired session.
var ErrNoSession = errors.New("console: no session")

// Identity is the signed-in user the console impersonates.
type Identity struct {
	Username string    `json:"u"`
	Groups   []string  `json:"g,omitempty"`
	Expiry   time.Time `json:"e"`
}

// Authenticator resolves the signed-in identity of a request.
type Authenticator interface {
	Identity(r *http.Request) (Identity, error)
}

type loginState struct {
	State     string `json:"s"`
	Verifier  string `json:"v"`
	Connector string `json:"c,omitempty"`
}

type loginNonce struct {
	Nonce string `json:"n"`
}

type provider struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
}

// Auth signs users in with the OIDC authorization code flow, using PKCE
// (S256), state and nonce, and keeps the identity in an encrypted cookie.
type Auth struct {
	cfg          Config
	key          []byte
	clientSecret string
	provider     atomic.Pointer[provider]
	now          func() time.Time
}

// NewAuth loads the session key and client secret and starts OIDC discovery.
// An unreachable issuer does not fail startup: discovery is retried every ten
// seconds until ctx is done, and Ready reports when it has succeeded.
func NewAuth(ctx context.Context, cfg Config) (*Auth, error) {
	if cfg.IssuerURL == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, errors.New("console: --oidc-issuer-url, --oidc-client-id and --redirect-url are required")
	}
	if cfg.UsernameClaim == "" {
		return nil, errors.New("console: --username-claim must not be empty")
	}
	key, err := sessionKey(ctx, cfg.SessionKeyFile)
	if err != nil {
		return nil, err
	}
	a := &Auth{cfg: cfg, key: key, now: time.Now}
	if cfg.ClientSecretFile != "" {
		secret, err := os.ReadFile(cfg.ClientSecretFile)
		if err != nil {
			return nil, fmt.Errorf("console: read client secret: %w", err)
		}
		a.clientSecret = strings.TrimSpace(string(secret))
	}
	if err := a.discover(ctx); err != nil {
		ctrllog.FromContext(ctx).Info("OIDC discovery failed; retrying", "issuer", cfg.IssuerURL, "error", redact.String(err.Error()))
		go a.retryDiscovery(ctx)
	}
	return a, nil
}

// sessionKey reads the 32-byte key from path or, without one, generates a
// random key held only in memory.
func sessionKey(ctx context.Context, path string) ([]byte, error) {
	if path == "" {
		key := make([]byte, SessionKeySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		ctrllog.FromContext(ctx).Info("Generated an in-memory session key because --session-key-file is not set; sessions will not survive a restart, and replicas will not share them")
		return key, nil
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("console: read session key: %w", err)
	}
	if len(key) != SessionKeySize {
		return nil, fmt.Errorf("console: session key file holds %d bytes, want exactly %d", len(key), SessionKeySize)
	}
	return key, nil
}

// Ready reports whether OIDC discovery has succeeded.
func (a *Auth) Ready() bool { return a.provider.Load() != nil }

func (a *Auth) discover(ctx context.Context) error {
	p, err := oidc.NewProvider(ctx, a.cfg.IssuerURL)
	if err != nil {
		return err
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if a.cfg.GroupsClaim != "" {
		scopes = append(scopes, "groups")
	}
	a.provider.Store(&provider{
		oauth: oauth2.Config{
			ClientID:     a.cfg.ClientID,
			ClientSecret: a.clientSecret,
			RedirectURL:  a.cfg.RedirectURL,
			Endpoint:     p.Endpoint(),
			Scopes:       scopes,
		},
		verifier: p.Verifier(&oidc.Config{ClientID: a.cfg.ClientID}),
	})
	return nil
}

func (a *Auth) retryDiscovery(ctx context.Context) {
	log := ctrllog.FromContext(ctx)
	ticker := time.NewTicker(discoveryRetry)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.discover(ctx); err != nil {
				log.Info("OIDC discovery failed; retrying", "issuer", a.cfg.IssuerURL, "error", redact.String(err.Error()))
				continue
			}
			log.Info("Completed OIDC discovery", "issuer", a.cfg.IssuerURL)
			return
		}
	}
}

// Start redirects to the issuer's authorization endpoint, for
// GET /auth/start?connector=.
func (a *Auth) Start(w http.ResponseWriter, r *http.Request) {
	p := a.provider.Load()
	if p == nil {
		a.fail(w, http.StatusServiceUnavailable, "unavailable", "the identity provider is not reachable yet")
		return
	}
	connector := r.URL.Query().Get("connector")
	if connector != "" && !slices.Contains(a.cfg.Connectors, connector) {
		a.fail(w, http.StatusBadRequest, "connector", "unknown sign-in connector")
		return
	}
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	sealedState, err := seal(a.key, loginState{State: state, Verifier: verifier, Connector: connector})
	if err != nil {
		a.fail(w, http.StatusInternalServerError, "internal", "could not start sign-in")
		return
	}
	sealedNonce, err := seal(a.key, loginNonce{Nonce: nonce})
	if err != nil {
		a.fail(w, http.StatusInternalServerError, "internal", "could not start sign-in")
		return
	}
	expires := a.now().Add(loginWindow)
	http.SetCookie(w, a.cookie(StateCookie, sealedState, "/auth", expires))
	http.SetCookie(w, a.cookie(NonceCookie, sealedNonce, "/auth", expires))
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce)}
	if connector != "" {
		opts = append(opts, oauth2.SetAuthURLParam("connector_id", connector))
	}
	http.Redirect(w, r, p.oauth.AuthCodeURL(state, opts...), http.StatusFound)
}

// Callback completes sign-in, for GET /auth/callback. It checks the state,
// redeems the code with the PKCE verifier, verifies the ID token's signature,
// issuer, audience, expiry and nonce, and sets the session cookie.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	p := a.provider.Load()
	if p == nil {
		a.fail(w, http.StatusServiceUnavailable, "unavailable", "the identity provider is not reachable yet")
		return
	}
	q := r.URL.Query()
	if q.Get("error") != "" {
		a.fail(w, http.StatusBadRequest, "denied", "the identity provider refused sign-in: "+q.Get("error"))
		return
	}
	var st loginState
	var nc loginNonce
	if !a.readCookie(r, StateCookie, &st) || !a.readCookie(r, NonceCookie, &nc) || st.State == "" || nc.Nonce == "" {
		a.fail(w, http.StatusBadRequest, "expired", "the sign-in expired or was started in another browser")
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		a.fail(w, http.StatusBadRequest, "state", "the sign-in state does not match")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), callbackTimeout)
	defer cancel()
	token, err := p.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		a.fail(w, http.StatusBadRequest, "exchange", "the authorization code was refused: "+err.Error())
		return
	}
	rawID, _ := token.Extra("id_token").(string)
	if rawID == "" {
		a.fail(w, http.StatusBadRequest, "token", "the identity provider returned no ID token")
		return
	}
	idToken, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		a.fail(w, http.StatusBadRequest, "token", "the ID token could not be verified: "+err.Error())
		return
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(nc.Nonce)) != 1 {
		a.fail(w, http.StatusBadRequest, "nonce", "the ID token nonce does not match")
		return
	}
	id, code, err := a.identityFromToken(idToken)
	if err != nil {
		a.fail(w, code, "claims", err.Error())
		return
	}
	sealed, err := seal(a.key, id)
	if err != nil {
		a.fail(w, http.StatusInternalServerError, "internal", "could not create the session")
		return
	}
	if len(sealed) > maxSessionCookie {
		a.fail(w, http.StatusBadRequest, "groups", errTooManyGroups)
		return
	}
	http.SetCookie(w, a.cookie(StateCookie, "", "/auth", time.Unix(0, 0)))
	http.SetCookie(w, a.cookie(NonceCookie, "", "/auth", time.Unix(0, 0)))
	http.SetCookie(w, a.cookie(SessionCookie, sealed, "/", id.Expiry))
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// identityFromToken maps the configured claims to an Identity, refusing an
// empty username and any system: identity.
func (a *Auth) identityFromToken(idToken *oidc.IDToken) (Identity, int, error) {
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, http.StatusBadRequest, fmt.Errorf("the ID token claims could not be read: %w", err)
	}
	username, _ := claims[a.cfg.UsernameClaim].(string)
	if username == "" {
		return Identity{}, http.StatusBadRequest, fmt.Errorf("the ID token has no %q claim; set --username-claim to a claim the issuer sends", a.cfg.UsernameClaim)
	}
	if a.cfg.UsernameClaim == emailClaim {
		if verified, ok := claims[emailVerifiedClaim].(bool); ok && !verified {
			return Identity{}, http.StatusForbidden, errors.New("the ID token's email is not verified")
		}
	}
	id := Identity{Username: a.cfg.UsernamePrefix + username, Expiry: idToken.Expiry}
	if a.cfg.GroupsClaim != "" {
		for _, g := range stringsClaim(claims[a.cfg.GroupsClaim]) {
			id.Groups = append(id.Groups, a.cfg.GroupsPrefix+g)
		}
	}
	if err := checkIdentity(id); err != nil {
		return Identity{}, http.StatusForbidden, err
	}
	return id, 0, nil
}

func stringsClaim(v any) []string {
	switch v := v.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// checkIdentity refuses identities the console must never impersonate.
func checkIdentity(id Identity) error {
	if id.Username == "" {
		return errors.New("an empty username cannot be impersonated")
	}
	if strings.HasPrefix(id.Username, systemPrefix) {
		return fmt.Errorf("the username %q is a system: identity, which the console never impersonates", id.Username)
	}
	for _, g := range id.Groups {
		if strings.HasPrefix(g, systemPrefix) {
			return fmt.Errorf("the group %q is a system: group, which the console never impersonates", g)
		}
	}
	return nil
}

// Logout clears the session, for POST /logout.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, a.cookie(SessionCookie, "", "/", time.Unix(0, 0)))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Identity returns the request's signed-in identity, or ErrNoSession.
func (a *Auth) Identity(r *http.Request) (Identity, error) {
	var id Identity
	if !a.readCookie(r, SessionCookie, &id) || !a.now().Before(id.Expiry) || checkIdentity(id) != nil {
		return Identity{}, ErrNoSession
	}
	return id, nil
}

func (a *Auth) readCookie(r *http.Request, name string, v any) bool {
	c, err := r.Cookie(name)
	return err == nil && open(a.key, c.Value, v) == nil
}

func (a *Auth) cookie(name, value, path string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		HttpOnly: true,
		Secure:   !a.cfg.InsecureCookies,
		SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}

// fail answers a failed sign-in step with its status code and a page that
// returns the browser to the login page, which shows "Sign-in failed".
func (a *Auth) fail(w http.ResponseWriter, code int, reason, message string) {
	target := "/login?error=" + url.QueryEscape(reason)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="3;url=%s"><title>Sign-in failed</title></head>`+
		`<body><h1>Sign-in failed</h1><p>%s</p><p><a href="%s">Return to sign in</a></p></body></html>`,
		html.EscapeString(target), html.EscapeString(redact.String(message)), html.EscapeString(target))
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
