package transport

import (
	"strings"

	utls "github.com/refraction-networking/utls"
)

// GetTLSProfile returns the utls ClientHelloID for the given profile string.
func GetTLSProfile(profile string) utls.ClientHelloID {
	switch strings.ToLower(profile) {
	case "chrome":
		return utls.HelloChrome_Auto
	case "firefox":
		return utls.HelloFirefox_Auto
	case "safari":
		return utls.HelloSafari_Auto
	case "ios":
		return utls.HelloIOS_Auto
	case "random":
		return utls.HelloRandomized
	default:
		return utls.HelloChrome_Auto
	}
}
