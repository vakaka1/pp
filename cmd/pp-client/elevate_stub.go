//go:build !windows

package main

func ensureElevatedForFullTunnel() (bool, error) {
	return false, nil
}
