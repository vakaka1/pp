//go:build windows

package main

import (
<<<<<<< HEAD
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
=======
	"os"
	"strings"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func ensureAdmin(log *zap.Logger) bool {
	if isAdmin() {
		return true
	}

	log.Info("Requesting administrator privileges for full-tunnel mode...")

	verb := "runas"
	exe, err := os.Executable()
	if err != nil {
		log.Error("failed to get executable path for elevation", zap.Error(err))
		return false
	}
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	verbPtr, _ := syscall.UTF16PtrFromString(verb)
	exePtr, _ := syscall.UTF16PtrFromString(exe)
	cwdPtr, _ := syscall.UTF16PtrFromString(cwd)
	argPtr, _ := syscall.UTF16PtrFromString(args)

	var showCmd int32 = 1 // SW_NORMAL

	err = windows.ShellExecute(0, verbPtr, exePtr, argPtr, cwdPtr, showCmd)
	if err != nil {
		log.Error("elevation failed", zap.Error(err))
		return false
	}

	// Exit the current non-elevated process because the new elevated one is launched.
	os.Exit(0)
	return true
>>>>>>> 146e723784e0497a73dcbc4f9e11904208153623
}
