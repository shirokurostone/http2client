package main

import (
	"encoding/binary"
	"io"
)

type FrameType uint8

const (
	DATA          FrameType = 0x00
	HEADERS       FrameType = 0x01
	PRIORITY      FrameType = 0x02
	RST_STREAM    FrameType = 0x03
	SETTINGS      FrameType = 0x04
	PUSH_PROMISE  FrameType = 0x05
	PING          FrameType = 0x06
	GOAWAY        FrameType = 0x07
	WINDOW_UPDATE FrameType = 0x08
	CONTINUATION  FrameType = 0x09
)

type Flags uint8

const (
	ACK Flags = 0x01
)

type FrameHeader struct {
	Length           uint32
	Type             FrameType
	Flags            byte
	StreamIdentifier uint32
}

func (h *FrameHeader) Serialize() []byte {
	var output [9]byte
	var tmp [4]byte

	binary.BigEndian.PutUint32(tmp[:], h.Length)
	copy(output[0:3], tmp[1:4])
	output[3] = byte(h.Type)
	output[4] = h.Flags
	binary.BigEndian.PutUint32(output[5:9], h.StreamIdentifier&0x7fffffff)
	return output[:]
}

func (h *FrameHeader) Deserialize(input []byte) error {
	var tmp [4]byte

	tmp[0] = 0
	copy(tmp[1:4], input[0:3])

	h.Length = binary.BigEndian.Uint32(tmp[:])
	h.Type = FrameType(input[3])
	h.Flags = input[4]
	h.StreamIdentifier = binary.BigEndian.Uint32(input[5:9]) & 0x7fffffff

	return nil
}

func (h *FrameHeader) Size() int {
	return 9
}

type SettingsPayload struct {
	Identifier uint16
	Value      uint32
}

func (p *SettingsPayload) Serialize() []byte {
	var output [6]byte
	var tmp [4]byte

	binary.BigEndian.PutUint16(tmp[0:2], p.Identifier)
	copy(output[0:2], tmp[0:2])
	binary.BigEndian.PutUint32(tmp[0:4], p.Value)
	copy(output[2:6], tmp[0:4])

	return output[:]
}

func (p *SettingsPayload) Deserialize(input []byte) error {
	p.Identifier = binary.BigEndian.Uint16(input[0:2])
	p.Value = binary.BigEndian.Uint32(input[2:6])
	return nil
}

func (p *SettingsPayload) Size() int {
	return 6
}

type HeaderPayload struct {
	PadLength           byte
	E                   byte
	StreamDependency    uint32
	Weight              byte
	HeaderBlockFragment []byte
}

func (h *HeaderPayload) Serialize() []byte {
	output := make([]byte, len(h.HeaderBlockFragment)+4)
	binary.BigEndian.PutUint32(output[0:4], uint32(h.E&0x01<<31)|h.StreamDependency)
	copy(output[4:], h.HeaderBlockFragment)
	return output
}

func (h *HeaderPayload) Deserialize(input []byte, padded bool, priority bool) error {
	i := 0
	if padded {
		h.PadLength = input[i]
		i++
	} else {
		h.PadLength = 0
	}

	tmp := binary.BigEndian.Uint32(input[i : i+4])
	h.StreamDependency = tmp & 0x7fffffff
	h.E = byte((tmp >> 31) & 0x01)
	i += 4

	if priority {
		h.Weight = input[i]
		i++
	}

	h.HeaderBlockFragment = input[i : len(input)-i-int(h.PadLength)]
	return nil
}

type SettingsParameter uint16

const (
	SETTINGS_HEADER_TABLE_SIZE      SettingsParameter = 0x1
	SETTINGS_ENABLE_PUSH            SettingsParameter = 0x2
	SETTINGS_MAX_CONCURRENT_STREAMS SettingsParameter = 0x3
	SETTINGS_INITIAL_WINDOW_SIZE    SettingsParameter = 0x4
	SETTINGS_MAX_FRAME_SIZE         SettingsParameter = 0x5
	SETTINGS_MAX_HEADER_LIST_SIZE   SettingsParameter = 0x6
)

type FixedLengthSerializable interface {
	Serialize() []byte
	Deserialize(input []byte) error
	Size() int
}

func ReadFrom(r io.Reader, s FixedLengthSerializable) (int64, error) {
	buf := make([]byte, s.Size())
	n, err := io.ReadFull(r, buf[:])
	if err != nil {
		return int64(n), err
	}

	err = s.Deserialize(buf[:])
	return int64(n), err
}

func WriteTo(w io.Writer, s FixedLengthSerializable) (int64, error) {
	n, err := w.Write(s.Serialize())
	return int64(n), err
}

type Frame interface {
	GetType() FrameType
}

type FrameBase struct {
	Header FrameHeader
}

func (f *FrameBase) GetType() FrameType {
	return f.Header.Type
}

type SettingsFrame struct {
	FrameBase
	Payload []SettingsPayload
}

type UnknownFrame struct {
	FrameBase
	Payload []byte
}

func ReadFrame(reader io.Reader) (Frame, error) {
	var header FrameHeader

	_, err := ReadFrom(reader, &header)
	if err != nil {
		return nil, err
	}

	switch header.Type {
	case SETTINGS:
		var frame SettingsFrame
		frame.Header = header

		var p SettingsPayload
		for i := 0; i < int(frame.Header.Length)/p.Size(); i++ {
			var payload SettingsPayload
			_, err := ReadFrom(reader, &payload)
			if err != nil {
				return nil, err
			}
			frame.Payload = append(frame.Payload, payload)
		}
		return &frame, nil
	default:
		var frame UnknownFrame
		frame.Header = header
		frame.Payload = make([]byte, frame.Header.Length)
		_, err := io.ReadFull(reader, frame.Payload)
		if err != nil {
			return nil, err
		}

		return &frame, nil
	}

}
