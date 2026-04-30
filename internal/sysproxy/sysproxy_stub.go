//go:build !windows

package sysproxy

func Enable(httpAddr string) error {
	return nil
}

func Disable() error {
	return nil
}
