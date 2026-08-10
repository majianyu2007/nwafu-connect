package zctcpip

import "testing"

func TestPacketValidationRejectsInvalidHeaderLengths(t *testing.T) {
	validIPv4 := make(IPv4Packet, IPv4HeaderSize)
	validIPv4[0] = IPv4Version<<4 | IPv4HeaderSize/4
	validIPv4.SetTotalLength(IPv4HeaderSize)
	if !validIPv4.Valid() {
		t.Fatal("well-formed minimum IPv4 packet was rejected")
	}

	for name, packet := range map[string]IPv4Packet{
		"short":              make(IPv4Packet, IPv4HeaderSize-1),
		"wrong version":      append(IPv4Packet(nil), validIPv4...),
		"zero header length": append(IPv4Packet(nil), validIPv4...),
		"truncated total":    append(IPv4Packet(nil), validIPv4...),
	} {
		switch name {
		case "wrong version":
			packet[0] = 6<<4 | IPv4HeaderSize/4
		case "zero header length":
			packet[0] = IPv4Version << 4
		case "truncated total":
			packet.SetTotalLength(IPv4HeaderSize + 1)
		}
		if packet.Valid() {
			t.Errorf("%s IPv4 packet was accepted", name)
		}
	}
}

func TestTransportValidationRejectsInvalidDeclaredLengths(t *testing.T) {
	tcpPacket := make(TCPPacket, TCPHeaderSize)
	tcpPacket[12] = byte(TCPHeaderSize/4) << 4
	if !tcpPacket.Valid() {
		t.Fatal("well-formed minimum TCP packet was rejected")
	}
	tcpPacket[12] = byte((TCPHeaderSize+4)/4) << 4
	if tcpPacket.Valid() {
		t.Fatal("TCP packet with a truncated declared header was accepted")
	}

	udpPacket := make(UDPPacket, UDPHeaderSize)
	udpPacket.SetLength(UDPHeaderSize)
	if !udpPacket.Valid() {
		t.Fatal("well-formed minimum UDP packet was rejected")
	}
	udpPacket.SetLength(0)
	if udpPacket.Valid() {
		t.Fatal("UDP packet with a zero declared length was accepted")
	}
}
