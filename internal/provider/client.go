package provider

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"resty.dev/v3"
)

// shellyUser is the only user name Shelly Gen2 firmware accepts for digest auth.
const shellyUser = "admin"

const passwordEnvVar = "SHELLY_PASSWORD"

var errUnauthorized = errors.New("authentication failed: the device rejected the password (HTTP 401). " +
	"Set the provider \"password\" (or " + passwordEnvVar + "), or the resource \"device_password\"")

// providerData is what the provider hands to every resource and data source.
type providerData struct {
	password string
}

// resolvePassword gives the configured value precedence over the environment.
func resolvePassword(configured types.String) string {
	if !configured.IsNull() && !configured.IsUnknown() {
		return configured.ValueString()
	}
	return os.Getenv(passwordEnvVar)
}

// client returns an HTTP client for the device at ip. A non-null override (the
// resource's device_password) wins over the provider-wide password. A nil
// receiver is valid: resources may run before the provider is configured, e.g.
// during validation.
func (p *providerData) client(ip string, override types.String) *resty.Client {
	password := ""
	if p != nil {
		password = p.password
	}
	if !override.IsNull() && !override.IsUnknown() {
		password = override.ValueString()
	}
	return newDeviceClient(ip, password)
}

func newDeviceClient(ip, password string) *resty.Client {
	c := resty.New().SetBaseURL("http://" + ip)
	if password != "" {
		c.SetTransport(&digestTransport{password: password, next: c.Transport()})
	}
	c.AddResponseMiddleware(func(_ *resty.Client, r *resty.Response) error {
		if r.StatusCode() == http.StatusUnauthorized {
			return errUnauthorized
		}
		return nil
	})
	return c
}

// authHA1 is the digest HA1 that Shelly.SetAuth expects: the hex SHA-256 of
// "admin:<device id>:<password>". The device ID is the digest realm.
func authHA1(deviceID, password string) string {
	sum := sha256.Sum256([]byte(shellyUser + ":" + deviceID + ":" + password))
	return hex.EncodeToString(sum[:])
}

// digestTransport does Shelly digest auth (RFC 7616, SHA-256, qop=auth). It
// sends the full request first and only answers a 401 challenge. resty's own
// digest transport sends a body-less probe first, which a device without a
// password (or Shelly.GetDeviceInfo) answers with an error, not a challenge.
type digestTransport struct {
	password string
	next     http.RoundTripper
}

func (t *digestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		_ = req.Body.Close()
	}
	withBody := func(r *http.Request) *http.Request {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return r
	}

	res, err := t.next.RoundTrip(withBody(req.Clone(req.Context())))
	if err != nil || res.StatusCode != http.StatusUnauthorized {
		return res, err
	}
	challenge := res.Header.Get("WWW-Authenticate")
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	auth, err := digestAuthorization(challenge, req.Method, req.URL.RequestURI(), t.password)
	if err != nil {
		return nil, err
	}
	retry := withBody(req.Clone(req.Context()))
	retry.Header.Set("Authorization", auth)
	return t.next.RoundTrip(retry)
}

func digestAuthorization(challenge, method, uri, password string) (string, error) {
	if !strings.HasPrefix(challenge, "Digest ") {
		return "", fmt.Errorf("unsupported authentication challenge %q", challenge)
	}
	params := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(challenge, "Digest "), ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
			params[strings.ToLower(k)] = strings.Trim(v, `"`)
		}
	}
	if alg := params["algorithm"]; alg != "" && !strings.EqualFold(alg, "SHA-256") {
		return "", fmt.Errorf("unsupported digest algorithm %q", alg)
	}
	realm, nonce := params["realm"], params["nonce"]

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	cnonce, nc := hex.EncodeToString(b), "00000001"
	h := func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
	response := h(strings.Join([]string{authHA1(realm, password), nonce, nc, cnonce, "auth", h(method + ":" + uri)}, ":"))

	return fmt.Sprintf(`Digest username=%q, realm=%q, nonce=%q, uri=%q, algorithm=SHA-256, qop=auth, nc=%s, cnonce=%q, response=%q`,
		shellyUser, realm, nonce, uri, nc, cnonce, response), nil
}
