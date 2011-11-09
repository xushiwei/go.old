// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin freebsd openbsd

// Routing sockets and messages

package syscall

import (
	"unsafe"
)

// Round the length of a raw sockaddr up to align it properly.
func rsaAlignOf(salen int) int {
	salign := sizeofPtr
	// NOTE: It seems like 64-bit Darwin kernel still requires 32-bit
	// aligned access to BSD subsystem.
	if darwinAMD64 {
		salign = 4
	}
	if salen == 0 {
		return salign
	}
	return (salen + salign - 1) & ^(salign - 1)
}

// RouteRIB returns routing information base, as known as RIB,
// which consists of network facility information, states and
// parameters.
func RouteRIB(facility, param int) ([]byte, int) {
	var (
		tab []byte
		e   int
	)

	mib := []_C_int{CTL_NET, AF_ROUTE, 0, 0, _C_int(facility), _C_int(param)}

	// Find size.
	n := uintptr(0)
	if e = sysctl(mib, nil, &n, nil, 0); e != 0 {
		return nil, e
	}
	if n == 0 {
		return nil, 0
	}

	tab = make([]byte, n)
	if e = sysctl(mib, &tab[0], &n, nil, 0); e != 0 {
		return nil, e
	}

	return tab[:n], 0
}

// RoutingMessage represents a routing message.
type RoutingMessage interface {
	sockaddr() []Sockaddr
}

const anyMessageLen = int(unsafe.Sizeof(anyMessage{}))

type anyMessage struct {
	Msglen  uint16
	Version uint8
	Type    uint8
}

// RouteMessage represents a routing message containing routing
// entries.
type RouteMessage struct {
	Header RtMsghdr
	Data   []byte
}

const rtaRtMask = RTA_DST | RTA_GATEWAY | RTA_NETMASK | RTA_GENMASK

func (m *RouteMessage) sockaddr() []Sockaddr {
	var (
		af  int
		sas [4]Sockaddr
	)

	buf := m.Data[:]
	for i := uint(0); i < RTAX_MAX; i++ {
		if m.Header.Addrs&rtaRtMask&(1<<i) == 0 {
			continue
		}
		rsa := (*RawSockaddr)(unsafe.Pointer(&buf[0]))
		switch i {
		case RTAX_DST, RTAX_GATEWAY:
			sa, e := anyToSockaddr((*RawSockaddrAny)(unsafe.Pointer(rsa)))
			if e != 0 {
				return nil
			}
			if i == RTAX_DST {
				af = int(rsa.Family)
			}
			sas[i] = sa
		case RTAX_NETMASK, RTAX_GENMASK:
			switch af {
			case AF_INET:
				rsa4 := (*RawSockaddrInet4)(unsafe.Pointer(&buf[0]))
				sa := new(SockaddrInet4)
				for j := 0; rsa4.Len > 0 && j < int(rsa4.Len)-int(unsafe.Offsetof(rsa4.Addr)); j++ {
					sa.Addr[j] = rsa4.Addr[j]
				}
				sas[i] = sa
			case AF_INET6:
				rsa6 := (*RawSockaddrInet6)(unsafe.Pointer(&buf[0]))
				sa := new(SockaddrInet6)
				for j := 0; rsa6.Len > 0 && j < int(rsa6.Len)-int(unsafe.Offsetof(rsa6.Addr)); j++ {
					sa.Addr[j] = rsa6.Addr[j]
				}
				sas[i] = sa
			}
		}
		buf = buf[rsaAlignOf(int(rsa.Len)):]
	}

	return sas[:]
}

// InterfaceMessage represents a routing message containing
// network interface entries.
type InterfaceMessage struct {
	Header IfMsghdr
	Data   []byte
}

func (m *InterfaceMessage) sockaddr() (sas []Sockaddr) {
	if m.Header.Addrs&RTA_IFP == 0 {
		return nil
	}
	sa, e := anyToSockaddr((*RawSockaddrAny)(unsafe.Pointer(&m.Data[0])))
	if e != 0 {
		return nil
	}
	return append(sas, sa)
}

// InterfaceAddrMessage represents a routing message containing
// network interface address entries.
type InterfaceAddrMessage struct {
	Header IfaMsghdr
	Data   []byte
}

const rtaIfaMask = RTA_IFA | RTA_NETMASK | RTA_BRD

func (m *InterfaceAddrMessage) sockaddr() (sas []Sockaddr) {
	if m.Header.Addrs&rtaIfaMask == 0 {
		return nil
	}

	buf := m.Data[:]
	for i := uint(0); i < RTAX_MAX; i++ {
		if m.Header.Addrs&rtaIfaMask&(1<<i) == 0 {
			continue
		}
		rsa := (*RawSockaddr)(unsafe.Pointer(&buf[0]))
		switch i {
		case RTAX_IFA:
			sa, e := anyToSockaddr((*RawSockaddrAny)(unsafe.Pointer(rsa)))
			if e != 0 {
				return nil
			}
			sas = append(sas, sa)
		case RTAX_NETMASK, RTAX_BRD:
			// nothing to do
		}
		buf = buf[rsaAlignOf(int(rsa.Len)):]
	}

	return sas
}

// ParseRoutingMessage parses buf as routing messages and returns
// the slice containing the RoutingMessage interfaces.
func ParseRoutingMessage(buf []byte) (msgs []RoutingMessage, errno int) {
	for len(buf) >= anyMessageLen {
		any := (*anyMessage)(unsafe.Pointer(&buf[0]))
		if any.Version != RTM_VERSION {
			return nil, EINVAL
		}
		msgs = append(msgs, any.toRoutingMessage(buf))
		buf = buf[any.Msglen:]
	}
	return msgs, 0
}

// ParseRoutingMessage parses msg's payload as raw sockaddrs and
// returns the slice containing the Sockaddr interfaces.
func ParseRoutingSockaddr(msg RoutingMessage) (sas []Sockaddr, errno int) {
	return append(sas, msg.sockaddr()...), 0
}
