// Package lan carries races over the local network: a TCP server the host
// runs, the client every racer (the host included) connects with, and the
// multicast beacon that lets rooms be found without typing an address.
package lan

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/dawsonxiong/thock/internal/content"
)

// BasePort is the first port a host tries. It moves up through Ports ports if
// that one is taken, so two rooms can run on one machine.
const (
	BasePort = 47300
	Ports    = 8
)

// A room code is two words from the 1k list, like "velvet-orbit". It spells the
// host's address: the last two octets of its IPv4 address and which of the
// Ports ports it took, 19 bits in all, as two digits in a base as large as the
// word list. Only words of four letters or more are used, so a code never
// reads as "a-the". A guest fills in the first two octets from its own
// address, which holds on any home or office network, where everyone shares
// a /16 or smaller.
//
// Codes are mostly matched against the beacons of rooms already found; the
// arithmetic is the fallback for networks that drop multicast.

// codeSpace is how many values a code must spell.
const codeSpace = 1 << 16 * Ports

func vocabulary() ([]string, error) {
	v, err := content.Vocabulary(content.List1k)
	if err != nil {
		return nil, err
	}
	out := v[:0]
	for _, w := range v {
		if len(w) >= 4 && strings.Trim(w, "abcdefghijklmnopqrstuvwxyz") == "" {
			out = append(out, w)
		}
	}
	if len(out)*len(out) < codeSpace {
		return nil, fmt.Errorf("the 1k list has only %d words long enough for codes", len(out))
	}
	return out, nil
}

// Code spells an address as a room code.
func Code(addr netip.AddrPort) (string, error) {
	ip := addr.Addr().Unmap()
	if !ip.Is4() {
		return "", errors.New("room codes need an IPv4 address")
	}
	off := int(addr.Port()) - BasePort
	if off < 0 || off >= Ports {
		return "", fmt.Errorf("port %d is outside the room code range", addr.Port())
	}
	dict, err := vocabulary()
	if err != nil {
		return "", err
	}
	b := ip.As4()
	n := (int(b[2])<<8|int(b[3]))*Ports + off
	return dict[n/len(dict)] + "-" + dict[n%len(dict)], nil
}

// Decode turns a room code back into an address, taking the network half of
// the address from local.
func Decode(code string, local netip.Addr) (netip.AddrPort, error) {
	parts := strings.FieldsFunc(strings.ToLower(code), func(r rune) bool {
		return r == '-' || r == ' ' || r == '.'
	})
	if len(parts) != 2 {
		return netip.AddrPort{}, fmt.Errorf("%q is not a room code: it should be two words, like velvet-orbit", code)
	}
	dict, err := vocabulary()
	if err != nil {
		return netip.AddrPort{}, err
	}
	index := func(w string) int {
		for i, d := range dict {
			if d == w {
				return i
			}
		}
		return -1
	}
	hi, lo := index(parts[0]), index(parts[1])
	if hi < 0 || lo < 0 {
		return netip.AddrPort{}, fmt.Errorf("%q is not a room code", code)
	}
	n := hi*len(dict) + lo
	if n >= codeSpace {
		return netip.AddrPort{}, fmt.Errorf("%q is not a room code", code)
	}
	local = local.Unmap()
	if !local.Is4() {
		return netip.AddrPort{}, errors.New("room codes need an IPv4 address on this machine")
	}
	b := local.As4()
	host := n / Ports
	ip := netip.AddrFrom4([4]byte{b[0], b[1], byte(host >> 8), byte(host)})
	return netip.AddrPortFrom(ip, uint16(BasePort+n%Ports)), nil
}

// LocalIP is the address other machines on the network reach this one at: the
// first IPv4 address on an interface that is up and not loopback, preferring
// private ranges, which is where a LAN lives.
func LocalIP() (netip.Addr, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return netip.Addr{}, false
	}
	var fallback netip.Addr
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			pfx, err := netip.ParsePrefix(a.String())
			if err != nil {
				continue
			}
			ip := pfx.Addr().Unmap()
			if !ip.Is4() || ip.IsLinkLocalUnicast() {
				continue
			}
			if ip.IsPrivate() {
				return ip, true
			}
			if !fallback.IsValid() {
				fallback = ip
			}
		}
	}
	return fallback, fallback.IsValid()
}
