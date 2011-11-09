// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd linux openbsd windows

// UDP sockets

package net

import (
	"bytes"
	"os"
	"syscall"
)

func sockaddrToUDP(sa syscall.Sockaddr) Addr {
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		return &UDPAddr{sa.Addr[0:], sa.Port}
	case *syscall.SockaddrInet6:
		return &UDPAddr{sa.Addr[0:], sa.Port}
	}
	return nil
}

func (a *UDPAddr) family() int {
	if a == nil || len(a.IP) <= IPv4len {
		return syscall.AF_INET
	}
	if a.IP.To4() != nil {
		return syscall.AF_INET
	}
	return syscall.AF_INET6
}

func (a *UDPAddr) sockaddr(family int) (syscall.Sockaddr, error) {
	return ipToSockaddr(family, a.IP, a.Port)
}

func (a *UDPAddr) toAddr() sockaddr {
	if a == nil { // nil *UDPAddr
		return nil // nil interface
	}
	return a
}

// UDPConn is the implementation of the Conn and PacketConn
// interfaces for UDP network connections.
type UDPConn struct {
	fd *netFD
}

func newUDPConn(fd *netFD) *UDPConn { return &UDPConn{fd} }

func (c *UDPConn) ok() bool { return c != nil && c.fd != nil }

// Implementation of the Conn interface - see Conn for documentation.

// Read implements the net.Conn Read method.
func (c *UDPConn) Read(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	return c.fd.Read(b)
}

// Write implements the net.Conn Write method.
func (c *UDPConn) Write(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	return c.fd.Write(b)
}

// Close closes the UDP connection.
func (c *UDPConn) Close() error {
	if !c.ok() {
		return os.EINVAL
	}
	err := c.fd.Close()
	c.fd = nil
	return err
}

// LocalAddr returns the local network address.
func (c *UDPConn) LocalAddr() Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.laddr
}

// RemoteAddr returns the remote network address, a *UDPAddr.
func (c *UDPConn) RemoteAddr() Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.raddr
}

// SetTimeout implements the net.Conn SetTimeout method.
func (c *UDPConn) SetTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setTimeout(c.fd, nsec)
}

// SetReadTimeout implements the net.Conn SetReadTimeout method.
func (c *UDPConn) SetReadTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setReadTimeout(c.fd, nsec)
}

// SetWriteTimeout implements the net.Conn SetWriteTimeout method.
func (c *UDPConn) SetWriteTimeout(nsec int64) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setWriteTimeout(c.fd, nsec)
}

// SetReadBuffer sets the size of the operating system's
// receive buffer associated with the connection.
func (c *UDPConn) SetReadBuffer(bytes int) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setReadBuffer(c.fd, bytes)
}

// SetWriteBuffer sets the size of the operating system's
// transmit buffer associated with the connection.
func (c *UDPConn) SetWriteBuffer(bytes int) error {
	if !c.ok() {
		return os.EINVAL
	}
	return setWriteBuffer(c.fd, bytes)
}

// UDP-specific methods.

// ReadFromUDP reads a UDP packet from c, copying the payload into b.
// It returns the number of bytes copied into b and the return address
// that was on the packet.
//
// ReadFromUDP can be made to time out and return an error with Timeout() == true
// after a fixed time limit; see SetTimeout and SetReadTimeout.
func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *UDPAddr, err error) {
	if !c.ok() {
		return 0, nil, os.EINVAL
	}
	n, sa, err := c.fd.ReadFrom(b)
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		addr = &UDPAddr{sa.Addr[0:], sa.Port}
	case *syscall.SockaddrInet6:
		addr = &UDPAddr{sa.Addr[0:], sa.Port}
	}
	return
}

// ReadFrom implements the net.PacketConn ReadFrom method.
func (c *UDPConn) ReadFrom(b []byte) (n int, addr Addr, err error) {
	if !c.ok() {
		return 0, nil, os.EINVAL
	}
	n, uaddr, err := c.ReadFromUDP(b)
	return n, uaddr.toAddr(), err
}

// WriteToUDP writes a UDP packet to addr via c, copying the payload from b.
//
// WriteToUDP can be made to time out and return
// an error with Timeout() == true after a fixed time limit;
// see SetTimeout and SetWriteTimeout.
// On packet-oriented connections, write timeouts are rare.
func (c *UDPConn) WriteToUDP(b []byte, addr *UDPAddr) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	sa, err1 := addr.sockaddr(c.fd.family)
	if err1 != nil {
		return 0, &OpError{Op: "write", Net: "udp", Addr: addr, Err: err1}
	}
	return c.fd.WriteTo(b, sa)
}

// WriteTo implements the net.PacketConn WriteTo method.
func (c *UDPConn) WriteTo(b []byte, addr Addr) (n int, err error) {
	if !c.ok() {
		return 0, os.EINVAL
	}
	a, ok := addr.(*UDPAddr)
	if !ok {
		return 0, &OpError{"writeto", "udp", addr, os.EINVAL}
	}
	return c.WriteToUDP(b, a)
}

// DialUDP connects to the remote address raddr on the network net,
// which must be "udp", "udp4", or "udp6".  If laddr is not nil, it is used
// as the local address for the connection.
func DialUDP(net string, laddr, raddr *UDPAddr) (c *UDPConn, err error) {
	switch net {
	case "udp", "udp4", "udp6":
	default:
		return nil, UnknownNetworkError(net)
	}
	if raddr == nil {
		return nil, &OpError{"dial", "udp", nil, errMissingAddress}
	}
	fd, e := internetSocket(net, laddr.toAddr(), raddr.toAddr(), syscall.SOCK_DGRAM, 0, "dial", sockaddrToUDP)
	if e != nil {
		return nil, e
	}
	return newUDPConn(fd), nil
}

// ListenUDP listens for incoming UDP packets addressed to the
// local address laddr.  The returned connection c's ReadFrom
// and WriteTo methods can be used to receive and send UDP
// packets with per-packet addressing.
func ListenUDP(net string, laddr *UDPAddr) (c *UDPConn, err error) {
	switch net {
	case "udp", "udp4", "udp6":
	default:
		return nil, UnknownNetworkError(net)
	}
	if laddr == nil {
		return nil, &OpError{"listen", "udp", nil, errMissingAddress}
	}
	fd, e := internetSocket(net, laddr.toAddr(), nil, syscall.SOCK_DGRAM, 0, "dial", sockaddrToUDP)
	if e != nil {
		return nil, e
	}
	return newUDPConn(fd), nil
}

// BindToDevice binds a UDPConn to a network interface.
func (c *UDPConn) BindToDevice(device string) error {
	if !c.ok() {
		return os.EINVAL
	}
	c.fd.incref()
	defer c.fd.decref()
	return os.NewSyscallError("setsockopt", syscall.BindToDevice(c.fd.sysfd, device))
}

// File returns a copy of the underlying os.File, set to blocking mode.
// It is the caller's responsibility to close f when finished.
// Closing c does not affect f, and closing f does not affect c.
func (c *UDPConn) File() (f *os.File, err error) { return c.fd.dup() }

// JoinGroup joins the IP multicast group named by addr on ifi,
// which specifies the interface to join.  JoinGroup uses the
// default multicast interface if ifi is nil.
func (c *UDPConn) JoinGroup(ifi *Interface, addr IP) error {
	if !c.ok() {
		return os.EINVAL
	}
	ip := addr.To4()
	if ip != nil {
		return joinIPv4GroupUDP(c, ifi, ip)
	}
	return joinIPv6GroupUDP(c, ifi, addr)
}

// LeaveGroup exits the IP multicast group named by addr on ifi.
func (c *UDPConn) LeaveGroup(ifi *Interface, addr IP) error {
	if !c.ok() {
		return os.EINVAL
	}
	ip := addr.To4()
	if ip != nil {
		return leaveIPv4GroupUDP(c, ifi, ip)
	}
	return leaveIPv6GroupUDP(c, ifi, addr)
}

func joinIPv4GroupUDP(c *UDPConn, ifi *Interface, ip IP) error {
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}
	if err := setIPv4InterfaceToJoin(mreq, ifi); err != nil {
		return &OpError{"joinipv4group", "udp", &IPAddr{ip}, err}
	}
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptIPMreq(c.fd.sysfd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)); err != nil {
		return &OpError{"joinipv4group", "udp", &IPAddr{ip}, err}
	}
	return nil
}

func leaveIPv4GroupUDP(c *UDPConn, ifi *Interface, ip IP) error {
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}
	if err := setIPv4InterfaceToJoin(mreq, ifi); err != nil {
		return &OpError{"leaveipv4group", "udp", &IPAddr{ip}, err}
	}
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptIPMreq(c.fd.sysfd, syscall.IPPROTO_IP, syscall.IP_DROP_MEMBERSHIP, mreq)); err != nil {
		return &OpError{"leaveipv4group", "udp", &IPAddr{ip}, err}
	}
	return nil
}

func setIPv4InterfaceToJoin(mreq *syscall.IPMreq, ifi *Interface) error {
	if ifi == nil {
		return nil
	}
	ifat, err := ifi.Addrs()
	if err != nil {
		return err
	}
	for _, ifa := range ifat {
		if x := ifa.(*IPAddr).IP.To4(); x != nil {
			copy(mreq.Interface[:], x)
			break
		}
	}
	if bytes.Equal(mreq.Multiaddr[:], IPv4zero) {
		return os.EINVAL
	}
	return nil
}

func joinIPv6GroupUDP(c *UDPConn, ifi *Interface, ip IP) error {
	mreq := &syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], ip)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(c.fd.sysfd, syscall.IPPROTO_IPV6, syscall.IPV6_JOIN_GROUP, mreq)); err != nil {
		return &OpError{"joinipv6group", "udp", &IPAddr{ip}, err}
	}
	return nil
}

func leaveIPv6GroupUDP(c *UDPConn, ifi *Interface, ip IP) error {
	mreq := &syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], ip)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(c.fd.sysfd, syscall.IPPROTO_IPV6, syscall.IPV6_LEAVE_GROUP, mreq)); err != nil {
		return &OpError{"leaveipv6group", "udp", &IPAddr{ip}, err}
	}
	return nil
}
