package dhcp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
)

// DHCP message types (option 53).
const (
	Discover = 1
	Offer    = 2
	Request  = 3
	Decline  = 4
	Ack      = 5
	Nak      = 6
	Release  = 7
	Inform   = 8
)

// Option codes used by this server.
const (
	OptPad            = 0
	OptSubnetMask     = 1
	OptRouter         = 3
	OptDNS            = 6
	OptDomainName     = 15
	OptBroadcast      = 28
	OptVendorSpecific = 43 // PXE vendor extensions
	OptRequestedIP    = 50
	OptLeaseTime      = 51
	OptMessageType    = 53
	OptServerID       = 54
	OptParamReqList   = 55
	OptVendorClass    = 60
	OptClientID       = 61
	OptTFTPServer     = 66
	OptBootFile       = 67
	OptUserClass      = 77
	OptClientArch     = 93
	OptClientUUID     = 97
	OptEnd            = 255
)

// magicCookie is the 4-byte sentinel that marks the start of the options area.
const magicCookie = 0x63825363

var ErrShort = errors.New("dhcp: packet too short")

// Packet is a hand-rolled BOOTP/DHCP message: 236-byte fixed header + cookie + TLV options.
// We do not use a library here because ProxyDHCP needs byte-level control over the
// option set (no lease, no mask, extra PXE options) and hand-rolling is ~150 lines.
type Packet struct {
	Op     byte // 1 = BOOTREQUEST, 2 = BOOTREPLY
	HType  byte
	HLen   byte
	Hops   byte
	XID    uint32
	Secs   uint16
	Flags  uint16
	CIAddr [4]byte // client IP (already configured)
	YIAddr [4]byte // "your" IP (offered address)
	SIAddr [4]byte // next server / boot server
	GIAddr [4]byte // relay agent
	CHAddr [16]byte
	SName  [64]byte
	File   [128]byte

	Options map[byte][]byte
}

// Parse decodes a raw DHCP datagram.
func Parse(b []byte) (*Packet, error) {
	if len(b) < 240 {
		return nil, ErrShort
	}
	if binary.BigEndian.Uint32(b[236:240]) != magicCookie {
		return nil, errors.New("dhcp: bad magic cookie (not a DHCP packet)")
	}
	p := &Packet{
		Op:      b[0],
		HType:   b[1],
		HLen:    b[2],
		Hops:    b[3],
		XID:     binary.BigEndian.Uint32(b[4:8]),
		Secs:    binary.BigEndian.Uint16(b[8:10]),
		Flags:   binary.BigEndian.Uint16(b[10:12]),
		Options: map[byte][]byte{},
	}
	copy(p.CIAddr[:], b[12:16])
	copy(p.YIAddr[:], b[16:20])
	copy(p.SIAddr[:], b[20:24])
	copy(p.GIAddr[:], b[24:28])
	copy(p.CHAddr[:], b[28:44])
	copy(p.SName[:], b[44:108])
	copy(p.File[:], b[108:236])

	for i := 240; i < len(b); {
		code := b[i]
		i++
		switch code {
		case OptPad:
			continue
		case OptEnd:
			return p, nil
		}
		if i >= len(b) {
			return p, nil
		}
		l := int(b[i])
		i++
		if i+l > len(b) {
			return nil, errors.New("dhcp: option overruns packet")
		}
		v := make([]byte, l)
		copy(v, b[i:i+l])
		i += l
		if _, dup := p.Options[code]; !dup {
			p.Options[code] = v
		}
	}
	return p, nil
}

// Marshal encodes the packet with the RFC 951 minimum length of 300 bytes.
func (p *Packet) Marshal() []byte {
	b := make([]byte, 240)
	b[0], b[1], b[2], b[3] = p.Op, p.HType, p.HLen, p.Hops
	binary.BigEndian.PutUint32(b[4:8], p.XID)
	binary.BigEndian.PutUint16(b[8:10], p.Secs)
	binary.BigEndian.PutUint16(b[10:12], p.Flags)
	copy(b[12:16], p.CIAddr[:])
	copy(b[16:20], p.YIAddr[:])
	copy(b[20:24], p.SIAddr[:])
	copy(b[24:28], p.GIAddr[:])
	copy(b[28:44], p.CHAddr[:])
	copy(b[44:108], p.SName[:])
	copy(b[108:236], p.File[:])
	binary.BigEndian.PutUint32(b[236:240], magicCookie)

	codes := make([]int, 0, len(p.Options))
	for c := range p.Options {
		codes = append(codes, int(c))
	}
	sort.Ints(codes)
	for _, c := range codes {
		if c == OptPad || c == OptEnd {
			continue
		}
		v := p.Options[byte(c)]
		if len(v) > 255 {
			v = v[:255]
		}
		b = append(b, byte(c), byte(len(v)))
		b = append(b, v...)
	}
	b = append(b, OptEnd)
	for len(b) < 300 {
		b = append(b, 0)
	}
	return b
}

// ---- option helpers ----

// ensureOptions 保证 Options 非 nil，零值 Packet 也能直接写入选项。
func (p *Packet) ensureOptions() map[byte][]byte {
	if p.Options == nil {
		p.Options = make(map[byte][]byte)
	}
	return p.Options
}

func (p *Packet) SetOpt(code byte, v []byte) { p.ensureOptions()[code] = v }

func (p *Packet) SetByte(code byte, v byte) { p.ensureOptions()[code] = []byte{v} }

func (p *Packet) SetU32(code byte, v uint32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	p.ensureOptions()[code] = b
}

func (p *Packet) SetIP(code byte, ip net.IP) {
	if ip4 := ip.To4(); ip4 != nil {
		p.ensureOptions()[code] = []byte{ip4[0], ip4[1], ip4[2], ip4[3]}
	}
}

func (p *Packet) SetString(code byte, s string) {
	if s == "" {
		return
	}
	p.ensureOptions()[code] = []byte(s)
}

func (p *Packet) Get(code byte) []byte { return p.Options[code] }

func (p *Packet) MessageType() byte {
	if v := p.Options[OptMessageType]; len(v) == 1 {
		return v[0]
	}
	return 0
}

func (p *Packet) VendorClass() string {
	return strings.TrimRight(string(p.Options[OptVendorClass]), "\x00")
}

func (p *Packet) UserClass() string {
	return strings.TrimRight(string(p.Options[OptUserClass]), "\x00")
}

// ClientArch returns the RFC 4578 client architecture (option 93).
// 0 = BIOS x86, 6 = UEFI x86, 7 = UEFI x64, 9 = UEFI x64 (alternate), 10/11 = ARM.
func (p *Packet) ClientArch() (uint16, bool) {
	v := p.Options[OptClientArch]
	if len(v) < 2 {
		return 0, false
	}
	return binary.BigEndian.Uint16(v[:2]), true
}

func (p *Packet) RequestedIP() net.IP {
	if v := p.Options[OptRequestedIP]; len(v) == 4 {
		return net.IP{v[0], v[1], v[2], v[3]}
	}
	return nil
}

// IsPXEClient reports whether the vendor class starts with "PXEClient" (option 60).
func (p *Packet) IsPXEClient() bool {
	return strings.HasPrefix(strings.ToUpper(p.VendorClass()), "PXECLIENT")
}

// IsIPXE reports whether the client identifies as iPXE/gPXE via option 77 user class.
func (p *Packet) IsIPXE() bool {
	uc := strings.ToLower(p.UserClass())
	return strings.Contains(uc, "ipxe") || strings.Contains(uc, "gpxe")
}

// ClientMAC formats chaddr as aa:bb:cc:dd:ee:ff.
func (p *Packet) ClientMAC() net.HardwareAddr {
	n := int(p.HLen)
	if n <= 0 || n > 16 {
		n = 6
	}
	return net.HardwareAddr(p.CHAddr[:n])
}

func TypeName(t byte) string {
	switch t {
	case Discover:
		return "DHCPDISCOVER"
	case Offer:
		return "DHCPOFFER"
	case Request:
		return "DHCPREQUEST"
	case Decline:
		return "DHCPDECLINE"
	case Ack:
		return "DHCPACK"
	case Nak:
		return "DHCPNAK"
	case Release:
		return "DHCPRELEASE"
	case Inform:
		return "DHCPINFORM"
	}
	return fmt.Sprintf("type(%d)", t)
}
