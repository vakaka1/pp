//go:build linux

package ppweb

import (
	"time"

	"golang.org/x/sys/unix"
)

func systemUptime() (time.Duration, bool) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0, false
	}
	return time.Duration(info.Uptime) * time.Second, true
}
