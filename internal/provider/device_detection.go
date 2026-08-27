package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Gen1StatusResponse represents the Gen1 device /status endpoint response
type Gen1StatusResponse struct {
	MAC     string `json:"mac"`
	Fw      string `json:"fw"`
	Version string `json:"new_version"`
}

// DeviceVersion represents the detected device generation and info
type DeviceVersion struct {
	Generation int // 1 for Gen1, 2 for Gen2
	MAC        string
	Firmware   string
	Error      error
}

// DetectDeviceVersion attempts to detect if a device is Gen1 or Gen2
func DetectDeviceVersion(ip string) *DeviceVersion {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}

	// Try Gen2 RPC API first
	resp, err := client.Get("http://" + ip + "/rpc/Shelly.GetDeviceInfo")
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Parse Gen2 response
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if mac, ok := result["mac"].(string); ok {
				if ver, ok := result["ver"].(string); ok {
					return &DeviceVersion{
						Generation: 2,
						MAC:        mac,
						Firmware:   ver,
					}
				}
			}
		}
	}

	// Try Gen1 /status endpoint
	resp, err = client.Get("http://" + ip + "/status")
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		var gen1 Gen1StatusResponse
		if err := json.Unmarshal(body, &gen1); err == nil {
			fw := gen1.Version
			if fw == "" {
				fw = gen1.Fw
			}
			return &DeviceVersion{
				Generation: 1,
				MAC:        gen1.MAC,
				Firmware:   fw,
			}
		}
	}

	return &DeviceVersion{
		Generation: 0,
		Error:      fmt.Errorf("unable to detect device generation at %s", ip),
	}
}

// IsGen2Device checks if a device supports Gen2 RPC API
func IsGen2Device(ip string) bool {
	version := DetectDeviceVersion(ip)
	return version.Generation == 2
}

// BuildDeviceCompatibilityError creates a helpful error message for incompatible devices
func BuildDeviceCompatibilityError(ip string, version *DeviceVersion) string {
	if version.Error != nil {
		return fmt.Sprintf(
			"Failed to detect device type at %s: %v\n\n"+
				"This operation requires a Shelly device/API that is supported by this resource.\n"+
				"Your device at %s does not appear to support the required RPC API endpoints.\n\n"+
				"The provider now includes partial Gen1 support for selected resources.\n"+
				"For resources without Gen1 support, use compatible Gen2 devices (Shelly Plus/Pro series).",
			ip, version.Error, ip)
	}

	if version.Generation == 1 {
		return fmt.Sprintf(
			"Device at %s is a Shelly Gen1 device (Firmware: %s, MAC: %s)\n\n"+
				"This specific resource currently requires Gen2 RPC capabilities.\n\n"+
				"Gen1 devices that were detected:\n"+
				"  - Shelly 1/1PM\n"+
				"  - Shelly 2/2.5\n"+
				"  - Shelly 4/4Pro\n"+
				"  - Shelly Plug/Plug S\n"+
				"  - Shelly EM/3EM\n\n"+
				"Try using a resource that has Gen1 support, or extend this resource with Gen1 endpoint mappings.",
			ip, version.Firmware, version.MAC)
	}

	return ""
}
