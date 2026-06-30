//go:build !linux && !windows

package ppweb

import "time"

func systemUptime() (time.Duration, bool) {
	return 0, false
}
