package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	gen1RequestTimeout = 30 * time.Second
	gen1RetryCount     = 7
	gen1RetryBaseDelay = 3 * time.Second
)

func gen1GetWithRetry(u string) (*http.Response, error) {
	client := &http.Client{Timeout: gen1RequestTimeout}
	var lastErr error
	for attempt := 1; attempt <= gen1RetryCount; attempt++ {
		resp, err := client.Get(u)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt < gen1RetryCount {
			time.Sleep(time.Duration(attempt) * gen1RetryBaseDelay)
		}
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", gen1RetryCount, lastErr)
}

type gen1SettingsResponse struct {
	Device struct {
		MAC  string `json:"mac"`
		Mode string `json:"mode"`
	} `json:"device"`
	Name                   string   `json:"name"`
	Fw                     string   `json:"fw"`
	Mode                   string   `json:"mode"`
	Discoverable           *bool    `json:"discoverable"`
	Timezone               string   `json:"timezone"`
	Lat                    float64  `json:"lat"`
	Lng                    float64  `json:"lng"`
	EcoModeEnabled         *bool    `json:"eco_mode_enabled"`
	AllowCrossOrigin       *bool    `json:"allow_cross_origin"`
	LedStatusDisable       *bool    `json:"led_status_disable"`
	LongpushTime           *float64 `json:"longpush_time"`
	TzAutodetect           *bool    `json:"tzautodetect"`
	FavoritesEnabled       *bool    `json:"favorites_enabled"`
	PonWifiReset           *bool    `json:"pon_wifi_reset"`
	FactoryResetFromSwitch *bool    `json:"factory_reset_from_switch"`
	WifiAP                 struct {
		Enabled *bool  `json:"enabled"`
		SSID    string `json:"ssid"`
		Key     string `json:"key"`
	} `json:"wifi_ap"`
	WifiSta struct {
		Enabled    *bool  `json:"enabled"`
		SSID       string `json:"ssid"`
		Ipv4Method string `json:"ipv4_method"`
		IP         string `json:"ip"`
		Gw         string `json:"gw"`
		Mask       string `json:"mask"`
		DNS        string `json:"dns"`
	} `json:"wifi_sta"`
	ApRoaming struct {
		Enabled   *bool    `json:"enabled"`
		Threshold *float64 `json:"threshold"`
	} `json:"ap_roaming"`
	Mqtt struct {
		Enable *bool  `json:"enable"`
		Server string `json:"server"`
		User   string `json:"user"`
		ID     string `json:"id"`
	} `json:"mqtt"`
	Coiot struct {
		Enabled      *bool  `json:"enabled"`
		UpdatePeriod *int64 `json:"update_period"`
		Peer         string `json:"peer"`
	} `json:"coiot"`
	Cloud struct {
		Enabled   *bool `json:"enabled"`
		Connected *bool `json:"connected"`
	} `json:"cloud"`
	Sntp struct {
		Server string `json:"server"`
	} `json:"sntp"`
}

type gen1RelaySettingsResponse struct {
	Name         *string  `json:"name"`
	BtnType      *string  `json:"btn_type"`
	DefaultState *string  `json:"default_state"`
	AutoOn       *float64 `json:"auto_on"`
	AutoOff      *float64 `json:"auto_off"`
	MaxPower     *float64 `json:"max_power"`
}

type gen1RollerSettingsResponse struct {
	MaxtimeOpen    *float64 `json:"maxtime_open"`
	MaxtimeClose   *float64 `json:"maxtime_close"`
	DefaultState   *string  `json:"default_state"`
	Swap           *bool    `json:"swap"`
	SwapInputs     *bool    `json:"swap_inputs"`
	InputMode      *string  `json:"input_mode"`
	ButtonType     *string  `json:"button_type"`
	ButtonReverse  *int     `json:"btn_reverse"`
	ObstaclePower  *float64 `json:"obstacle_power"`
	ObstacleDelay  *float64 `json:"obstacle_delay"`
	ObstacleMode   *string  `json:"obstacle_mode"`
	ObstacleAction *string  `json:"obstacle_action"`
	SafetyMode     *string  `json:"safety_mode"`
	SafetyAction   *string  `json:"safety_action"`
	Positioning    *bool    `json:"positioning"`
}

type gen1FavoriteSettingsResponse struct {
	Name string `json:"name"`
	Pos  *int64 `json:"pos"`
}

func gen1HTTPGetJSON(path string, target any) error {
	var lastErr error
	for attempt := 1; attempt <= gen1RetryCount; attempt++ {
		if attempt > 1 {
			// Back off before retrying truncated / timed-out responses.
			time.Sleep(time.Duration(attempt) * gen1RetryBaseDelay)
		}
		client := &http.Client{Timeout: gen1RequestTimeout}
		resp, err := client.Get(path)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if err := json.Unmarshal(body, target); err != nil {
			// Truncated JSON means the device was overloaded – retry.
			lastErr = fmt.Errorf("failed to unmarshal response body: %w", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("request failed after %d attempts: %w", gen1RetryCount, lastErr)
}

func gen1GetSettings(ip string) (*gen1SettingsResponse, error) {
	out := &gen1SettingsResponse{}
	if err := gen1HTTPGetJSON("http://"+ip+"/settings", out); err != nil {
		return nil, err
	}
	return out, nil
}

func gen1GetRelaySettings(ip string, id int) (*gen1RelaySettingsResponse, error) {
	out := &gen1RelaySettingsResponse{}
	if err := gen1HTTPGetJSON("http://"+ip+"/settings/relay/"+strconv.Itoa(id), out); err != nil {
		return nil, err
	}
	return out, nil
}

func gen1GetRollerSettings(ip string, id int) (*gen1RollerSettingsResponse, error) {
	out := &gen1RollerSettingsResponse{}
	if err := gen1HTTPGetJSON("http://"+ip+"/settings/roller/"+strconv.Itoa(id), out); err != nil {
		return nil, err
	}
	return out, nil
}

func gen1GetFavoriteSettings(ip string, id int) (*gen1FavoriteSettingsResponse, error) {
	out := &gen1FavoriteSettingsResponse{}
	if err := gen1HTTPGetJSON("http://"+ip+"/settings/favorites/"+strconv.Itoa(id), out); err != nil {
		return nil, err
	}
	return out, nil
}

func gen1GetMode(ip string) (string, error) {
	settings, err := gen1GetSettings(ip)
	if err != nil {
		return "", err
	}
	if settings.Mode != "" {
		return strings.ToLower(settings.Mode), nil
	}
	if settings.Device.Mode != "" {
		return strings.ToLower(settings.Device.Mode), nil
	}
	return "", nil
}

// gen1DoSet fires a GET-style settings URL and retries on connection errors.
// The response body is drained but not parsed (Gen1 returns the full settings
// JSON which we don't need after a write).
func gen1DoSet(u string) error {
	var lastErr error
	for attempt := 1; attempt <= gen1RetryCount; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt) * gen1RetryBaseDelay)
		}
		client := &http.Client{Timeout: gen1RequestTimeout}
		resp, err := client.Get(u)
		if err != nil {
			lastErr = err
			continue
		}
		io.Copy(io.Discard, resp.Body) //nolint:errcheck
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}
	return fmt.Errorf("set request failed after %d attempts: %w", gen1RetryCount, lastErr)
}

func gen1SetSettings(ip string, q url.Values) error {
	if len(q) == 0 {
		return nil
	}
	return gen1DoSet("http://" + ip + "/settings?" + q.Encode())
}

func gen1SetRelaySettings(ip string, id int, q url.Values) error {
	if len(q) == 0 {
		return nil
	}
	return gen1DoSet("http://" + ip + "/settings/relay/" + strconv.Itoa(id) + "?" + q.Encode())
}

func gen1SetRollerSettings(ip string, id int, q url.Values) error {
	if len(q) == 0 {
		return nil
	}
	return gen1DoSet("http://" + ip + "/settings/roller/" + strconv.Itoa(id) + "?" + q.Encode())
}

func gen1SetFavoriteSettings(ip string, id int, q url.Values) error {
	if len(q) == 0 {
		return nil
	}
	return gen1DoSet("http://" + ip + "/settings/favorites/" + strconv.Itoa(id) + "?" + q.Encode())
}

func mapGen1BtnTypeToInMode(v string) string {
	switch strings.ToLower(v) {
	case "momentary":
		return "momentary"
	case "edge":
		return "flip"
	case "detached":
		return "detached"
	case "activation":
		return "activate"
	default:
		return "follow"
	}
}

func mapInModeToGen1BtnType(v string) string {
	switch strings.ToLower(v) {
	case "momentary":
		return "momentary"
	case "flip":
		return "edge"
	case "detached":
		return "detached"
	case "activate":
		return "activation"
	default:
		return "toggle"
	}
}

func mapGen1DefaultStateToInitialState(v string) string {
	switch strings.ToLower(v) {
	case "off":
		return "off"
	case "on":
		return "on"
	case "last":
		return "restore_last"
	case "switch":
		return "match_input"
	default:
		return "restore_last"
	}
}

func mapInitialStateToGen1DefaultState(v string) string {
	switch strings.ToLower(v) {
	case "off":
		return "off"
	case "on":
		return "on"
	case "restore_last":
		return "last"
	case "match_input":
		return "switch"
	default:
		return "last"
	}
}

func mapGen1RollerInputToInMode(inputMode string) string {
	switch strings.ToLower(inputMode) {
	case "onebutton":
		return "single"
	case "openclose":
		return "dual"
	default:
		return "dual"
	}
}

func mapInModeToGen1RollerInput(v string) string {
	switch strings.ToLower(v) {
	case "single":
		return "onebutton"
	case "dual":
		return "openclose"
	default:
		return "openclose"
	}
}

func mapGen1RollerDefaultStateToInitialState(v string) string {
	switch strings.ToLower(v) {
	case "open":
		return "open"
	case "close", "closed":
		return "closed"
	case "stop":
		return "stop"
	default:
		return "stop"
	}
}

func mapInitialStateToGen1RollerDefaultState(v string) string {
	switch strings.ToLower(v) {
	case "open":
		return "open"
	case "closed":
		return "close"
	default:
		return "stop"
	}
}
