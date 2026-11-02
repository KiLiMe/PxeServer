package dhcp

import (
	"net"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	p := &Packet{Op: 1, HType: 1, HLen: 6, XID: 42, Flags: 0x8000, Options: map[byte][]byte{}}
	copy(p.CHAddr[:], net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	p.SetByte(OptMessageType, Discover)
	p.SetString(OptVendorClass, "PXEClient:Arch:00000:UNDI:002001")
	p.SetOpt(OptClientArch, []byte{0x00, 0x00})
	p.SetString(OptUserClass, "gPXE")

	b := p.Marshal()
	if len(b) < 300 {
		t.Fatalf("packet too short: %d", len(b))
	}
	q, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if q.XID != 42 || q.MessageType() != Discover || q.Op != 1 {
		t.Fatalf("header mismatch: %+v", q)
	}
	if q.ClientMAC().String() != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac mismatch: %s", q.ClientMAC())
	}
	if !q.IsPXEClient() || !q.IsIPXE() {
		t.Fatal("vendor/user class mismatch")
	}
	if a, _ := q.ClientArch(); a != 0 {
		t.Fatalf("arch mismatch: %d", a)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error for short packet")
	}
	b := make([]byte, 300)
	if _, err := Parse(b); err == nil {
		t.Fatal("expected error for missing magic cookie")
	}
}

func TestPaddingOption(t *testing.T) {
	// pad (0) options inside the stream must be skipped, not treated as TLV.
	p := &Packet{Op: 2, Options: map[byte][]byte{OptMessageType: {Offer}}}
	b := p.Marshal()
	q, err := Parse(append(b[:240], append([]byte{0, 0, OptMessageType, 1, Offer, OptEnd}, b[241:]...)...))
	if err != nil {
		t.Fatalf("pad handling: %v", err)
	}
	if q.MessageType() != Offer {
		t.Fatalf("expected OFFER after padding, got %d", q.MessageType())
	}
}
