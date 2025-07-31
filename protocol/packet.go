package protocol

import (
	"encoding/binary"
	"errors"
	"net"
)

type IPPacket struct {
	Raw      []byte
	Version  uint8
	Protocol uint8
	SrcIp    net.IP
	DstIp    net.IP
	Payload  []byte
}

func ParseIPPacket(data []byte) (*IPPacket, error) {
	if len(data) < 20 {
		return nil, errors.New("packet too short")
	}
	packet := &IPPacket{
		Raw: data,
	}
	packet.Version = data[0] >> 4
	if packet.Version == 4 {
		return ParseIPv4Packet(packet, data)
	} else if packet.Version == 6 {
		return ParseIPv6Packet(packet, data)
	}
	return nil, errors.New("invalid packet")
}

func ParseIPv4Packet(packet *IPPacket, data []byte) (*IPPacket, error) {
	if len(data) < 20 {
		return nil, errors.New("packet too short")
	}
	ihl := int((data[0] & 0x0F) * 4)
	if len(data) < ihl {
		return nil, errors.New("invalid IPv4 header length")
	}
	packet.Protocol = data[9]
	packet.SrcIp = data[12:16]
	packet.DstIp = data[16:20]
	if len(data) > ihl {
		packet.Payload = data[ihl:]
	}
	return packet, nil
}

func ParseIPv6Packet(packet *IPPacket, data []byte) (*IPPacket, error) {
	if len(data) < 40 {
		return nil, errors.New("packet too short")
	}
	packet.Protocol = data[6]
	packet.SrcIp = data[8:24]
	packet.DstIp = data[24:40]
	if len(data) > 40 {
		packet.Payload = data[40:]
	}
	return packet, nil
}

func isDNS(packet *IPPacket) bool {
	if packet.Protocol != 17 {
		return false
	}
	if len(packet.Payload) >= 4 {
		dstPort := binary.BigEndian.Uint16(packet.Payload[2:4])
		return dstPort == 53
	}
	return false
}

func (p *IPPacket) ProtocolName() string {
	switch p.Protocol {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return "Unknown"
	}
}
