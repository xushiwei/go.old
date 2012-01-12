// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux netbsd openbsd windows

// Unix domain sockets

package net

import (
	"os"
	"syscall"
)

func unixSocket(net string, laddr, raddr *UnixAddr, mode string) (fd *netFD, err error) {
	var proto int
	switch net {
	default:
		return nil, UnknownNetworkError(net)
	case "unix":
		proto = syscall.SOCK_STREAM
	case "unixgram":
		proto = syscall.SOCK_DGRAM
	case "unixpacket":
		proto = syscall.SOCK_SEQPACKET
	}

	var la, ra syscall.Sockaddr
	switch mode {
	default:
		panic("unixSocket mode " + mode)

	case "dial":
		if laddr != nil {
			la = &syscall.SockaddrUnix{Name: laddr.Name}
		}
		if raddr != nil {
			ra = &syscall.SockaddrUnix{Name: raddr.Name}
		} else if proto != syscall.SOCK_DGRAM || laddr == nil {
			return nil, &OpError{Op: mode, Net: net, Err: errMissingAddress}
		}

	case "listen":
		if laddr == nil {
			return nil, &OpError{mode, net, nil, errMissingAddress}
		}
		la = &syscall.SockaddrUnix{Name: laddr.Name}
		if raddr != nil {
			return nil, &OpError{Op: mode, Net: net, Addr: raddr, Err: &AddrError{Err: "unexpected remote address", Addr: raddr.String()}}
		}
	}

	f := sockaddrToUnix
	if proto == syscall.SOCK_DGRAM {
		f = sockaddrToUnixgram
	} else if proto == syscall.SOCK_SEQPACKET {
		f = sockaddrToUnixpacket
	}

	fd, oserr := socket(net, syscall.AF_UNIX, proto, 0, la, ra, f)
	if oserr != nil {
		goto Error
	}
	return fd, nil

Error:
	addr := raddr
	if mode == "listen" {
		addr = laddr
	}
	return nil, &OpError{Op: mode, Net: net, Addr: addr, Err: oserr}
}

func sockaddrToUnix(sa syscall.Sockaddr) Addr {
	if s, ok := sa.(*syscall.SockaddrUnix); ok {
		return &UnixAddr{s.Name, "unix"}
	}
	return nil
}

func sockaddrToUnixgram(sa syscall.Sockaddr) Addr {
	if s, ok := sa.(*syscall.SockaddrUnix); ok {
		return &UnixAddr{s.Name, "unixgram"}
	}
	return nil
}

func sockaddrToUnixpacket(sa syscall.Sockaddr) Addr {
	if s, ok := sa.(*syscall.SockaddrUnix); ok {
		return &UnixAddr{s.Name, "unixpacket"}
	}
	return nil
}

func protoToNet(proto int) string {
	switch proto {
	case syscall.SOCK_STREAM:
		return "unix"
	case syscall.SOCK_SEQPACKET:
		return "unixpacket"
	case syscall.SOCK_DGRAM:
		return "unixgram"
	default:
		panic("protoToNet unknown protocol")
	}
	return ""
}

// UnixConn is an implementation of the Conn interface
// for connections to Unix domain sockets.
type UnixConn struct {
	fd *netFD
}

func newUnixConn(fd *netFD) *UnixConn { return &UnixConn{fd} }

func (c *UnixConn) ok() bool { return c != nil && c.fd != nil }

// Implementation of the Conn interface - see Conn for documentation.

// Read implements the net.Conn Read method.
func (c *UnixConn) Read(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	return c.fd.Read(b)
}

// Write implements the net.Conn Write method.
func (c *UnixConn) Write(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	return c.fd.Write(b)
}

// Close closes the Unix domain connection.
func (c *UnixConn) Close() error {
	if !c.ok() {
		return os.EINVAL
	}
	err := c.fd.Close()
	c.fd = nil
	return err
}

// LocalAddr returns the local network address, a *UnixAddr.
// Unlike in other protocols, LocalAddr is usually nil for dialed connections.
func (c *UnixConn) LocalAddr() Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.laddr
}

// RemoteAddr returns the remote network address, a *UnixAddr.
// Unlike in other protocols, RemoteAddr is usually nil for connections
// accepted by a listener.
func (c *UnixConn) RemoteAddr() Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.raddr
}

// SetTimeout implements the net.Conn SetTimeout method.
func (c *UnixConn) SetTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setTimeout(c.fd, nsec)
}

// SetReadTimeout implements the net.Conn SetReadTimeout method.
func (c *UnixConn) SetReadTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setReadTimeout(c.fd, nsec)
}

// SetWriteTimeout implements the net.Conn SetWriteTimeout method.
func (c *UnixConn) SetWriteTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setWriteTimeout(c.fd, nsec)
}

// SetReadBuffer sets the size of the operating system's
// receive buffer associated with the connection.
func (c *UnixConn) SetReadBuffer(bytes int) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setReadBuffer(c.fd, bytes)
}

// SetWriteBuffer sets the size of the operating system's
// transmit buffer associated with the connection.
func (c *UnixConn) SetWriteBuffer(bytes int) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setWriteBuffer(c.fd, bytes)
}

// ReadFromUnix reads a packet from c, copying the payload into b.
// It returns the number of bytes copied into b and the return address
// that was on the packet.
//
// ReadFromUnix can be made to time out and return
// an error with Timeout() == true after a fixed time limit;
// see SetTimeout and SetReadTimeout.
func (c *UnixConn) ReadFromUnix(b []byte) (n int, addr *UnixAddr, err error) {
	if !c.ok() {
		return 0, nil, os.EINVAL
	}
	n, sa, err := c.fd.ReadFrom(b)
	switch sa := sa.(type) {
	case *syscall.SockaddrUnix:
		addr = &UnixAddr{sa.Name, protoToNet(c.fd.proto)}
	}
	return
}

// ReadFrom implements the net.PacketConn ReadFrom method.
func (c *UnixConn) ReadFrom(b []byte) (n int, addr Addr, err error) {
	if !c.ok() {
		return 0, nil, os.EINVAL
	}
	n, uaddr, err := c.ReadFromUnix(b)
	return n, uaddr.toAddr(), err
}

// WriteToUnix writes a packet to addr via c, copying the payload from b.
//
// WriteToUnix can be made to time out and return
// an error with Timeout() == true after a fixed time limit;
// see SetTimeout and SetWriteTimeout.
// On packet-oriented connections, write timeouts are rare.
func (c *UnixConn) WriteToUnix(b []byte, addr *UnixAddr) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	if addr.Net != protoToNet(c.fd.proto) {
		return 0, os.EAFNOSUPPORT
	}
	sa := &syscall.SockaddrUnix{Name: addr.Name}
	return c.fd.WriteTo(b, sa)
}

// WriteTo implements the net.PacketConn WriteTo method.
func (c *UnixConn) WriteTo(b []byte, addr Addr) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	a, ok := addr.(*UnixAddr)
	if !ok {
		return 0, &OpError{"writeto", "unix", addr, os.EINVAL}
	}
	return c.WriteToUnix(b, a)
}

func (c *UnixConn) ReadMsgUnix(b, oob []byte) (n, oobn, flags int, addr *UnixAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, os.EINVAL
	}
	n, oobn, flags, sa, err := c.fd.ReadMsg(b, oob)
	switch sa := sa.(type) {
	case *syscall.SockaddrUnix:
		addr = &UnixAddr{sa.Name, protoToNet(c.fd.proto)}
	}
	return
}

func (c *UnixConn) WriteMsgUnix(b, oob []byte, addr *UnixAddr) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, os.EINVAL
	}
	if addr != nil {
		if addr.Net != protoToNet(c.fd.proto) {
			return 0, 0, os.EAFNOSUPPORT
		}
		sa := &syscall.SockaddrUnix{Name: addr.Name}
		return c.fd.WriteMsg(b, oob, sa)
	}
	return c.fd.WriteMsg(b, oob, nil)
}

// File returns a copy of the underlying os.File, set to blocking mode.
// It is the caller's responsibility to close f when finished.
// Closing c does not affect f, and closing f does not affect c.
func (c *UnixConn) File() (f *os.File, err error) { return c.fd.dup() }

// DialUnix connects to the remote address raddr on the network net,
// which must be "unix" or "unixgram".  If laddr is not nil, it is used
// as the local address for the connection.
func DialUnix(net string, laddr, raddr *UnixAddr) (c *UnixConn, err error) {
	fd, e := unixSocket(net, laddr, raddr, "dial")
	if e != nil {
		return nil, e
	}
	return newUnixConn(fd), nil
}

// UnixListener is a Unix domain socket listener.
// Clients should typically use variables of type Listener
// instead of assuming Unix domain sockets.
type UnixListener struct {
	fd   *netFD
	path string
}

// ListenUnix announces on the Unix domain socket laddr and returns a Unix listener.
// Net must be "unix" (stream sockets).
func ListenUnix(net string, laddr *UnixAddr) (l *UnixListener, err error) {
	if net != "unix" && net != "unixgram" && net != "unixpacket" {
		return nil, UnknownNetworkError(net)
	}
	if laddr != nil {
		laddr = &UnixAddr{laddr.Name, net} // make our own copy
	}
	fd, err := unixSocket(net, laddr, nil, "listen")
	if err != nil {
		return nil, err
	}
	e1 := syscall.Listen(fd.sysfd, 8) // listenBacklog());
	if e1 != nil {
		closesocket(fd.sysfd)
		return nil, &OpError{Op: "listen", Net: "unix", Addr: laddr, Err: e1}
	}
	return &UnixListener{fd, laddr.Name}, nil
}

// AcceptUnix accepts the next incoming call and returns the new connection
// and the remote address.
func (l *UnixListener) AcceptUnix() (c *UnixConn, err error) {
	if l == nil || l.fd == nil {
		return nil, os.EINVAL
	}
	fd, e := l.fd.accept(sockaddrToUnix)
	if e != nil {
		return nil, e
	}
	c = newUnixConn(fd)
	return c, nil
}

// Accept implements the Accept method in the Listener interface;
// it waits for the next call and returns a generic Conn.
func (l *UnixListener) Accept() (c Conn, err error) {
	c1, err := l.AcceptUnix()
	if err != nil {
		return nil, err
	}
	return c1, nil
}

// Close stops listening on the Unix address.
// Already accepted connections are not closed.
func (l *UnixListener) Close() error {
	if l == nil || l.fd == nil {
		return os.EINVAL
	}

	// The operating system doesn't clean up
	// the file that announcing created, so
	// we have to clean it up ourselves.
	// There's a race here--we can't know for
	// sure whether someone else has come along
	// and replaced our socket name already--
	// but this sequence (remove then close)
	// is at least compatible with the auto-remove
	// sequence in ListenUnix.  It's only non-Go
	// programs that can mess us up.
	if l.path[0] != '@' {
		syscall.Unlink(l.path)
	}
	err := l.fd.Close()
	l.fd = nil
	return err
}

// Addr returns the listener's network address.
func (l *UnixListener) Addr() Addr { return l.fd.laddr }

// SetTimeout sets the deadline associated wuth the listener
func (l *UnixListener) SetTimeout(nsec int64) (err error) {
	if l == nil || l.fd == nil {
		return os.EINVAL
	}
	return setTimeout(l.fd, nsec)
}

// File returns a copy of the underlying os.File, set to blocking mode.
// It is the caller's responsibility to close f when finished.
// Closing c does not affect f, and closing f does not affect c.
func (l *UnixListener) File() (f *os.File, err error) { return l.fd.dup() }

// ListenUnixgram listens for incoming Unix datagram packets addressed to the
// local address laddr.  The returned connection c's ReadFrom
// and WriteTo methods can be used to receive and send UDP
// packets with per-packet addressing.  The network net must be "unixgram".
func ListenUnixgram(net string, laddr *UnixAddr) (c *UDPConn, err error) {
	switch net {
	case "unixgram":
	default:
		return nil, UnknownNetworkError(net)
	}
	if laddr == nil {
		return nil, &OpError{"listen", "unixgram", nil, errMissingAddress}
	}
	fd, e := unixSocket(net, laddr, nil, "listen")
	if e != nil {
		return nil, e
	}
	return newUDPConn(fd), nil
}
