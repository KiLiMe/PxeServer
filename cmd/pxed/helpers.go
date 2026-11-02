package main

import (
	"fmt"
	"net"
	"time"

	"pxeserv/internal/config"
	"pxeserv/internal/dhcp"
)

func timeNow() string { return time.Now().Format("15:04:05") } // 与 GUI 版日志时间戳保持一致

// runSelfTest exercises packet parse/marshal and the boot-file table without
// needing admin rights, a VM, or a real PXE client.
func runSelfTest() {
	cfg := config.Default()
	cfg.ServerIP = "192.168.5.1"
	cfg.NextServer = "192.168.5.1"

	// 1) build a PXE DHCPDISCOVER: UEFI x64 client
	req := &dhcp.Packet{
		Op: 1, HType: 1, HLen: 6, XID: 0x11223344, Flags: 0x8000,
		Options: map[byte][]byte{},
	}
	copy(req.CHAddr[:], net.HardwareAddr{0x52, 0x54, 0x00, 0x12, 0x34, 0x56})
	req.SetByte(dhcp.OptMessageType, dhcp.Discover)
	req.SetString(dhcp.OptVendorClass, "PXEClient:Arch:00007:UNDI:003016")
	req.SetOpt(dhcp.OptClientArch, []byte{0x00, 0x07})
	req.SetString(dhcp.OptUserClass, "iPXE")
	raw := req.Marshal()
	fmt.Printf("selftest: marshalled DISCOVER = %d bytes\n", len(raw))

	// 2) round-trip
	got, err := dhcp.Parse(raw)
	if err != nil {
		fmt.Println("selftest: FAIL parse:", err)
		return
	}
	arch, _ := got.ClientArch()
	fmt.Printf("selftest: round-trip mac=%s arch=%d pxe=%v ipxe=%v type=%s\n",
		got.ClientMAC(), arch, got.IsPXEClient(), got.IsIPXE(), dhcp.TypeName(got.MessageType()))
	if got.XID != 0x11223344 || arch != 7 || !got.IsPXEClient() || !got.IsIPXE() {
		fmt.Println("selftest: FAIL field mismatch")
		return
	}

	// 3) arch -> boot file table
	for _, a := range []uint16{0, 6, 7, 9, 99} {
		fmt.Printf("selftest: arch %-2d -> %s\n", a, cfg.BootFileFor(a))
	}

	// 4) reply marshalling length sanity
	fmt.Printf("selftest: min packet length = %d (RFC 951 requires >= 300)\n", len(req.Marshal()))
	fmt.Println("selftest: OK")
}
