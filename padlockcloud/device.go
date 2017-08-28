package padlockcloud

import (
	"fmt"
	"net/http"
)

var IOS_DEVICES = map[string]string{
	"iPhone1,1": "iPhone",
	"iPhone1,2": "iPhone 3G",
	"iPhone2,1": "iPhone 3GS",
	"iPhone3,1": "iPhone 4",
	"iPhone3,2": "iPhone 4",
	"iPhone3,3": "iPhone 4",
	"iPhone4,1": "iPhone 4S",
	"iPhone5,1": "iPhone 5",
	"iPhone5,2": "iPhone 5",
	"iPhone5,3": "iPhone 5C",
	"iPhone5,4": "iPhone 5C",
	"iPhone6,1": "iPhone 5S",
	"iPhone6,2": "iPhone 5S",
	"iPhone7,1": "iPhone 6 Plus",
	"iPhone7,2": "iPhone 6",
	"iPhone8,1": "iPhone 6S",
	"iPhone8,2": "iPhone 6S Plus",
	"iPhone8,3": "iPhone SE",
	"iPhone8,4": "iPhone SE",
	"iPhone9,1": "iPhone 7",
	"iPhone9,2": "iPhone 7 Plus",
	"iPhone9,3": "iPhone 7",
	"iPhone9,4": "iPhone 7 Plus",

	"iPod1,1": "iPod touch (1st Gen)",
	"iPod2,1": "iPod touch (2nd Gen)",
	"iPod3,1": "iPod touch (3rd Gen)",
	"iPod4,1": "iPod touch (4th Gen)",
	"iPod5,1": "iPod touch (5th Gen)",
	"iPod7,1": "iPod touch (6th Gen)",

	"iPad1,1":  "iPad (1st Gen)",
	"iPad1,2":  "iPad (1st Gen)",
	"iPad2,1":  "iPad (2nd Gen)",
	"iPad2,2":  "iPad (2nd Gen)",
	"iPad2,3":  "iPad (2nd Gen)",
	"iPad2,4":  "iPad (2nd Gen)",
	"iPad2,5":  "iPad mini (1st Gen)",
	"iPad2,6":  "iPad mini (1st Gen)",
	"iPad2,7":  "iPad mini (1st Gen)",
	"iPad3,1":  "iPad (3rd Gen)",
	"iPad3,2":  "iPad (3rd Gen)",
	"iPad3,3":  "iPad (3rd Gen)",
	"iPad3,4":  "iPad (4th Gen)",
	"iPad3,5":  "iPad (4th Gen)",
	"iPad3,6":  "iPad (4th Gen)",
	"iPad4,1":  "iPad Air (1st Gen)",
	"iPad4,2":  "iPad Air (1st Gen)",
	"iPad4,3":  "iPad Air",
	"iPad4,4":  "iPad mini (2nd Gen)",
	"iPad4,5":  "iPad mini (2nd Gen)",
	"iPad4,6":  "iPad mini (2nd Gen)",
	"iPad4,7":  "iPad mini (3rd Gen)",
	"iPad4,8":  "iPad mini (3rd Gen)",
	"iPad4,9":  "iPad mini (3rd Gen)",
	"iPad5,1":  "iPad mini (4th Gen)",
	"iPad5,2":  "iPad mini (4th Gen)",
	"iPad5,3":  "iPad Air (2nd Gen)",
	"iPad5,4":  "iPad Air (2nd Gen)",
	"iPad6,3":  "iPad Pro 9.7\"",
	"iPad6,4":  "iPad Pro 9.7\"",
	"iPad6,7":  "iPad Pro 12.9\" (1st Gen)",
	"iPad6,8":  "iPad Pro 12.9\" (1st Gen))",
	"iPad6,11": "iPad (5th Gen)",
	"iPad6,12": "iPad (5th Gen)",
	"iPad7,1":  "iPad Pro 12.9\" (2nd Gen)",
	"iPad7,2":  "iPad Pro 12.9\" (2nd Gen)",
	"iPad7,3":  "iPad Pro 10.5\"",
	"iPad7,4":  "iPad Pro 10.5\"",
}

func PlatformDisplayName(platform string) string {
	switch platform {
	case "darwin":
		return "MacOS"
	case "win32":
		return "Windows"
	default:
		return platform
	}
}

type Device struct {
	// Permanent fields - these are not going to change
	Platform     string `json:"platform"`
	UUID         string `json:"uuid"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	// Dynamic fields - these may be updated
	OSVersion  string `json:"osVersion"`
	HostName   string `json:"hostName"`
	AppVersion string `json:"appVersion"`
}

func (d *Device) Description() string {
	var desc string

	if desc = d.HostName; desc == "" {
		if d.Model != "" {
			if m, ok := IOS_DEVICES[d.Model]; ok {
				desc = m
			} else {
				desc = d.Model
			}
		} else {
			desc = PlatformDisplayName(d.Platform) + " Device"
		}
	}

	if d.Platform != "" && d.OSVersion != "" {
		desc = desc + fmt.Sprintf(" (%s %s)", PlatformDisplayName(d.Platform), d.OSVersion)
	}

	return desc
}

func (d *Device) UpdateFromRequest(r *http.Request) {
	if osVersion := r.Header.Get("X-Device-OS-Version"); osVersion != "" {
		d.OSVersion = osVersion
	}
	if hostName := r.Header.Get("X-Device-Hostname"); hostName != "" {
		d.HostName = hostName
	}
	var appVersion string
	if appVersion = r.Header.Get("X-Device-App-Version"); appVersion == "" {
		appVersion = r.Header.Get("X-Client-App-Version")
	}
	if appVersion != "" {
		d.AppVersion = appVersion
	}
}

func DeviceFromRequest(r *http.Request) *Device {
	var platform string
	if platform = r.Header.Get("X-Device-Platform"); platform == "" {
		if platform = r.Header.Get("X-Client-Platform"); platform == "" {
			return nil
		}
	}

	device := &Device{
		Platform:     platform,
		UUID:         r.Header.Get("X-Device-UUID"),
		Model:        r.Header.Get("X-Device-Model"),
		Manufacturer: r.Header.Get("X-Device-Manufacturer"),
	}

	device.UpdateFromRequest(r)

	return device
}
