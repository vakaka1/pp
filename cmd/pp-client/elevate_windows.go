//go:build windows

package main

import (
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
}
