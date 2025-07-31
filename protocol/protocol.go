package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
)

const (
	TypeHandshake    uint8 = 1
	TypeHandshakeAck uint8 = 2
	TypeKeepAlive    uint8 = 3
	TypeDisconnect   uint8 = 4
	TypeData         uint8 = 10
	TypeError        uint8 = 255
)

type Header struct {
	Type   uint8
	Length uint32
}

const HeaderSize = 5

type Message struct {
	Header Header
	Data   []byte
}

type HandshakeMsg struct {
	Version   uint8
	ClientIP  string
	SharedKey []byte
}

func NewMessage(msgType uint8, data []byte) *Message {
	return &Message{
		Header: Header{
			Type:   msgType,
			Length: uint32(len(data)),
		},
		Data: data,
	}
}

func WriteMessage(conn net.Conn, msg *Message) error {
	if err := binary.Write(conn, binary.BigEndian, msg.Header.Type); err != nil {
		return err
	}
	if err := binary.Write(conn, binary.BigEndian, msg.Header.Length); err != nil {
		return err
	}
	if msg.Header.Length > 0 {
		if _, err := conn.Write(msg.Data); err != nil {
			return err
		}
	}
	return nil
}

func ReadMessage(conn net.Conn) (*Message, error) {
	msg := &Message{}
	if err := binary.Read(conn, binary.BigEndian, &msg.Header.Type); err != nil {
		return nil, err
	}
	if err := binary.Read(conn, binary.BigEndian, &msg.Header.Length); err != nil {
		return nil, err
	}

	if msg.Header.Length > 1024*1024 {
		return nil, errors.New("too Large message")
	}

	if msg.Header.Length > 0 {
		msg.Data = make([]byte, msg.Header.Length)
		if _, err := io.ReadFull(conn, msg.Data); err != nil {
			return nil, err
		}
	}
	return msg, nil
}

func CreateHandshake(version uint8, clientIP string, sharedKey []byte) *Message {
	data := make([]byte, 1+len(clientIP)+1+len(sharedKey))
	data[0] = version
	data[1] = byte(len(clientIP))
	copy(data[2:], clientIP)
	copy(data[2+len(clientIP):], sharedKey)
	return NewMessage(TypeHandshake, data)
}

func ParseHandshake(data []byte) (*HandshakeMsg, error) {
	if len(data) < 2 {
		return nil, errors.New("invalid handshake packet")
	}
	version := data[0]
	IPLen := int(data[1])

	if len(data) < 2+IPLen {
		return nil, errors.New("invalid handshake packet")
	}
	clientIP := string(data[2 : 2+IPLen])
	sharedKey := data[2+IPLen:]

	return &HandshakeMsg{
		Version:   version,
		ClientIP:  clientIP,
		SharedKey: sharedKey,
	}, nil
}
