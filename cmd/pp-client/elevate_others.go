//go:build !windows

package main

import "go.uber.org/zap"

func ensureAdmin(log *zap.Logger) bool {
	return true
}
