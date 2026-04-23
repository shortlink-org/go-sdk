// Package freeport returns an ephemeral TCP port bound on localhost.
package freeport

import (
	"net"
)

// GetFreePort asks the kernel for a free open port that is ready to use.
func GetFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	tcpListener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = tcpListener.Close() //nolint:errcheck // best-effort close of ephemeral listener
	}()

	port, ok := tcpListener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, ErrNoFreePort
	}

	return port.Port, nil
}
