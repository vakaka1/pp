//go:build linux

package proxy

import (
	"fmt"
	"math/bits"
	"net"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const soOriginalDst = 80

// HandleTransparent handles a transparently redirected TCP connection and
// recovers the original upstream destination from the socket.
func HandleTransparent(conn net.Conn) (*Request, error) {
	target, err := originalDst(conn)
	if err != nil {
		return nil, err
	}
	return &Request{Target: target}, nil
}

func originalDst(conn net.Conn) (string, error) {
	sysConn, ok := conn.(syscall.Conn)
	if !ok {
		return "", fmt.Errorf("connection does not expose raw syscall access")
	}

	rawConn, err := sysConn.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("failed to access raw connection: %w", err)
	}

	var (
		target   string
		innerErr error
	)
	if err := rawConn.Control(func(fd uintptr) {
		target, innerErr = originalDstIPv4(fd)
	}); err != nil {
		return "", fmt.Errorf("failed to inspect redirected socket: %w", err)
	}
	if innerErr != nil {
		return "", innerErr
	}
	return target, nil
}

func originalDstIPv4(fd uintptr) (string, error) {
	var addr unix.RawSockaddrInet4
	size := uint32(unsafe.Sizeof(addr))

	_, _, errno := unix.Syscall6(
		unix.SYS_GETSOCKOPT,
		fd,
		uintptr(unix.SOL_IP),
		uintptr(soOriginalDst),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if errno != 0 {
		return "", fmt.Errorf("failed to read SO_ORIGINAL_DST: %w", errno)
	}
	if addr.Family != unix.AF_INET {
		return "", fmt.Errorf("unsupported redirected address family: %d", addr.Family)
	}

	ip := net.IP(addr.Addr[:]).String()
	port := bits.ReverseBytes16(addr.Port)
	return net.JoinHostPort(ip, strconv.Itoa(int(port))), nil
}
