//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

func ensureElevatedForFullTunnel() (bool, error) {
	elevated, err := isCurrentProcessElevated()
	if err != nil {
		return false, err
	}
	if elevated {
		return false, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to resolve executable path: %w", err)
	}

	params := windowsCommandLine(os.Args[1:])
	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return false, err
	}
	exePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return false, err
	}
	paramsPtr, err := windows.UTF16PtrFromString(params)
	if err != nil {
		return false, err
	}
	cwdPtr, err := windows.UTF16PtrFromString(mustGetwd())
	if err != nil {
		return false, err
	}

	if err := shellExecuteRunas(verbPtr, exePtr, paramsPtr, cwdPtr); err != nil {
		return false, fmt.Errorf("failed to request administrator privileges: %w", err)
	}
	fmt.Println("Administrator privileges requested; continuing in the elevated pp-client process.")
	return true, nil
}

func shellExecuteRunas(verbPtr, exePtr, paramsPtr, cwdPtr *uint16) error {
	ret, _, err := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(exePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(cwdPtr)),
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.Errno(ret)
	}
	return nil
}

func isCurrentProcessElevated() (bool, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false, fmt.Errorf("failed to open process token: %w", err)
	}
	defer token.Close()

	var elevation windows.TokenElevation
	var returnedLen uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevation)),
		uint32(unsafe.Sizeof(elevation)),
		&returnedLen,
	)
	if err != nil {
		return false, fmt.Errorf("failed to read process elevation: %w", err)
	}
	return elevation.TokenIsElevated != 0, nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func windowsCommandLine(args []string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		escaped = append(escaped, windowsEscapeArg(arg))
	}
	return strings.Join(escaped, " ")
}

func windowsEscapeArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}

	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
			continue
		}
		if backslashes > 0 {
			b.WriteString(strings.Repeat(`\`, backslashes))
			backslashes = 0
		}
		b.WriteRune(r)
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}
