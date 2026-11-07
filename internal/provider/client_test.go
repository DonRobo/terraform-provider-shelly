package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shelly "github.com/DonRobo/shelly-go"
	"github.com/DonRobo/shelly-go/components"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
)

const testDeviceID = "shellyplus1-test"

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// digestDevice mimics Shelly Gen2 digest auth: SHA-256, realm = device ID,
// and Shelly.GetDeviceInfo is open without credentials.
func digestDevice(t *testing.T, password string) *httptest.Server {
	t.Helper()
	ha1 := authHA1(testDeviceID, password)
	const nonce = "abc123"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "Shelly.GetDeviceInfo") {
			fmt.Fprintf(w, `{"id":1,"result":{"id":%q,"mac":"AABBCC","ver":"1.0.0","auth_en":true}}`, testDeviceID)
			return
		}
		p := map[string]string{}
		for _, part := range strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Digest "), ",") {
			if k, v, ok := strings.Cut(strings.TrimSpace(part), "="); ok {
				p[k] = strings.Trim(v, `"`)
			}
		}
		want := hexSHA256(strings.Join([]string{ha1, p["nonce"], p["nc"], p["cnonce"], p["qop"], hexSHA256(r.Method + ":" + p["uri"])}, ":"))
		if p["username"] != "admin" || p["response"] != want {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest qop="auth", realm=%q, nonce=%q, algorithm=SHA-256`, testDeviceID, nonce))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"id":1,"result":{"name":"relay"}}`)
	}))
}

func TestAuthHA1(t *testing.T) {
	require.Equal(t, hexSHA256("admin:"+testDeviceID+":secret"), authHA1(testDeviceID, "secret"))
}

func TestDeviceClientDigestAuth(t *testing.T) {
	srv := digestDevice(t, "secret")
	defer srv.Close()
	ip := strings.TrimPrefix(srv.URL, "http://")

	t.Run("correct password", func(t *testing.T) {
		c := newDeviceClient(ip, "secret")
		defer c.Close()
		_, _, err := (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
		require.NoError(t, err)
	})

	t.Run("incorrect password gives a clear error", func(t *testing.T) {
		c := newDeviceClient(ip, "wrong")
		defer c.Close()
		_, _, err := (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
		require.ErrorIs(t, err, errUnauthorized)
	})

	t.Run("no password gives a clear error", func(t *testing.T) {
		c := newDeviceClient(ip, "")
		defer c.Close()
		_, _, err := (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
		require.ErrorIs(t, err, errUnauthorized)
	})

	t.Run("device info needs no password", func(t *testing.T) {
		c := newDeviceClient(ip, "")
		defer c.Close()
		info, _, err := (&shelly.ShellyGetDeviceInfoRequest{}).Do(c)
		require.NoError(t, err)
		require.Equal(t, testDeviceID, info.ID)
	})
}

// A password must not break a device that has authentication off: the client
// sends the full request and the device answers it without a challenge.
func TestDeviceClientPasswordOnOpenDevice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			http.Error(w, "empty body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"result":{"name":"relay"}}`)
	}))
	defer srv.Close()

	c := newDeviceClient(strings.TrimPrefix(srv.URL, "http://"), "secret")
	defer c.Close()
	_, _, err := (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
	require.NoError(t, err)
}

func TestProviderDataClientPrecedence(t *testing.T) {
	srv := digestDevice(t, "device-pw")
	defer srv.Close()
	ip := strings.TrimPrefix(srv.URL, "http://")
	pd := &providerData{password: "provider-pw"}

	c := pd.client(ip, types.StringNull())
	_, _, err := (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
	c.Close()
	require.ErrorIs(t, err, errUnauthorized, "provider password is not the device password")

	c = pd.client(ip, types.StringValue("device-pw"))
	_, _, err = (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
	c.Close()
	require.NoError(t, err, "device_password overrides the provider password")

	var unconfigured *providerData
	c = unconfigured.client(ip, types.StringValue("device-pw"))
	_, _, err = (&components.SwitchGetConfigRequest{ID: 0}).Do(c)
	c.Close()
	require.NoError(t, err, "a nil provider data still works")
}

func TestResolvePassword(t *testing.T) {
	t.Setenv(passwordEnvVar, "from-env")
	require.Equal(t, "from-config", resolvePassword(types.StringValue("from-config")))
	require.Equal(t, "from-env", resolvePassword(types.StringNull()))
}

func TestPasswordAttributesAreSensitive(t *testing.T) {
	ctx := context.Background()

	var presp provider.SchemaResponse
	New("test")().Schema(ctx, provider.SchemaRequest{}, &presp)
	pw, ok := presp.Schema.Attributes["password"].(providerschema.StringAttribute)
	require.True(t, ok)
	require.True(t, pw.Sensitive, "provider password")

	for name, res := range map[string]resource.Resource{
		"shelly_auth":          NewAuthResource(),
		"shelly_switch_config": NewSwitchConfigResource(),
	} {
		var rresp resource.SchemaResponse
		res.Schema(ctx, resource.SchemaRequest{}, &rresp)
		attr := "device_password"
		if name == "shelly_auth" {
			attr = "password"
		}
		a, ok := rresp.Schema.Attributes[attr].(schema.StringAttribute)
		require.True(t, ok, name)
		require.True(t, a.Sensitive, name+"."+attr)
	}
}
