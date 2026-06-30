//go:build windows

package ppweb

import (
	"time"

	"golang.org/x/sys/windows"
)

var getTickCount64 = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetTickCount64")

func systemUptime() (time.Duration, bool) {
	milliseconds, _, err := getTickCount64.Call()
	if milliseconds == 0 && err != windows.ERROR_SUCCESS {
		return 0, false
	}
	return time.Duration(milliseconds) * time.Millisecond, true
}
