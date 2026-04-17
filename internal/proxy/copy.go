package proxy

import "io"

// Copy copies data bidirectionally.
func Copy(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}
