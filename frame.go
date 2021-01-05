package main

import (
	"encoding/binary"
	"io"
)

const HTTP2CoccectionPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

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
	ACK            Flags = 0x01
	END_STREAM     Flags = 0x01
	END_HEADERS    Flags = 0x04
	PADDED         Flags = 0x08
	FLAGS_PRIORITY Flags = 0x20
)

func (self Flags) Has(f Flags) bool {
	return self&f == f
}

type FrameHeader struct {
	Length           uint32
	Type             FrameType
	Flags            Flags
	StreamIdentifier uint32
}

func (h *FrameHeader) Serialize() []byte {
	var output [9]byte
	var tmp [4]byte

	binary.BigEndian.PutUint32(tmp[:], h.Length)
	copy(output[0:3], tmp[1:4])
	output[3] = byte(h.Type)
	output[4] = byte(h.Flags)
	binary.BigEndian.PutUint32(output[5:9], h.StreamIdentifier&0x7fffffff)
	return output[:]
}

func (h *FrameHeader) Deserialize(input []byte) error {
	var tmp [4]byte

	tmp[0] = 0
	copy(tmp[1:4], input[0:3])

	h.Length = binary.BigEndian.Uint32(tmp[:])
	h.Type = FrameType(input[3])
	h.Flags = Flags(input[4])
	h.StreamIdentifier = binary.BigEndian.Uint32(input[5:9]) & 0x7fffffff

	return nil
}

func (h *FrameHeader) Size() int {
	return 9
}

type SettingsParameter struct {
	Identifier uint16
	Value      uint32
}

type SettingsPayload struct {
	Parameters []SettingsParameter
}

func (p *SettingsParameter) Serialize() []byte {
	var output [6]byte
	var tmp [4]byte

	binary.BigEndian.PutUint16(tmp[0:2], p.Identifier)
	copy(output[0:2], tmp[0:2])
	binary.BigEndian.PutUint32(tmp[0:4], p.Value)
	copy(output[2:6], tmp[0:4])

	return output[:]
}

func (p *SettingsParameter) Deserialize(input []byte) error {
	p.Identifier = binary.BigEndian.Uint16(input[0:2])
	p.Value = binary.BigEndian.Uint32(input[2:6])
	return nil
}

func (p *SettingsParameter) Size() int {
	return 6
}

func (p *SettingsPayload) Serialize() []byte {
	var output []byte

	for i := 0; i < len(p.Parameters); i++ {
		output = append(output, p.Parameters[i].Serialize()...)
	}

	return output
}

func (p *SettingsPayload) Deserialize(input []byte) error {
	p.Parameters = make([]SettingsParameter, 0)
	for i := 0; i < len(input); {
		var param SettingsParameter
		param.Deserialize(input[i : i+param.Size()])
		p.Parameters = append(p.Parameters, param)
		i += param.Size()
	}

	return nil
}

type HeadersPayload struct {
	PadLength           byte
	E                   byte
	StreamDependency    uint32
	Weight              byte
	HeaderBlockFragment []byte
}

func (h *HeadersPayload) Serialize() []byte {
	output := make([]byte, len(h.HeaderBlockFragment))
	copy(output[0:], h.HeaderBlockFragment)
	return output
}

func (h *HeadersPayload) Deserialize(input []byte, padded bool, priority bool) error {
	i := 0
	if padded {
		h.PadLength = input[i]
		i++
	} else {
		h.PadLength = 0
	}

	if priority {
		tmp := binary.BigEndian.Uint32(input[i : i+4])
		h.StreamDependency = tmp & 0x7fffffff
		h.E = byte((tmp >> 31) & 0x01)
		i += 4

		h.Weight = input[i]
		i++
	}

	h.HeaderBlockFragment = input[i : len(input)-i-int(h.PadLength)]
	return nil
}

type GoawayPayload struct {
	LastStreamID        uint32
	ErrorCode           uint32
	AdditionalDebugData []byte
}

func (p *GoawayPayload) Deserialize(input []byte) error {

	tmp := binary.BigEndian.Uint32(input[0:4])
	p.LastStreamID = tmp & 0x7fffffff

	tmp = binary.BigEndian.Uint32(input[4:8])
	p.ErrorCode = tmp

	p.AdditionalDebugData = input[8:]

	return nil
}

type ErrorCode uint32

const (
	NO_ERROR            ErrorCode = 0x00
	PROTOCOL_ERROR      ErrorCode = 0x01
	INTERNAL_ERROR      ErrorCode = 0x02
	FLOW_CONTROL_ERROR  ErrorCode = 0x03
	SETTINGS_TIMEOUT    ErrorCode = 0x04
	STREAM_CLOSED       ErrorCode = 0x05
	FRAME_SIZE_ERROR    ErrorCode = 0x06
	REFUSED_STREAM      ErrorCode = 0x07
	CANCEL              ErrorCode = 0x08
	COMPRESSION_ERROR   ErrorCode = 0x09
	CONNECT_ERROR       ErrorCode = 0x0a
	ENHANCE_YOUR_CALM   ErrorCode = 0x0b
	INADEQUATE_SECURITY ErrorCode = 0x0c
	HTTP_1_1_REQUIRED   ErrorCode = 0x0d
)

type WindowUpdatePayload struct {
	WindowSizeIncrement uint32
}

func (p *WindowUpdatePayload) Size() int {
	return 4
}
func (p *WindowUpdatePayload) Serialize() []byte {
	var output [4]byte
	binary.BigEndian.PutUint32(output[0:4], p.WindowSizeIncrement&0x7fffffff)
	return output[:]
}

func (p *WindowUpdatePayload) Deserialize(input []byte) error {
	tmp := binary.BigEndian.Uint32(input[0:4])
	p.WindowSizeIncrement = tmp & 0x7fffffff
	return nil
}

type DataPayload struct {
	Data []byte
}

func (p *DataPayload) Serialize() []byte {
	return p.Data
}

func (p *DataPayload) Deserialize(input []byte) error {
	p.Data = input
	return nil
}

type SettingsParameterType uint16

const (
	SETTINGS_HEADER_TABLE_SIZE      SettingsParameterType = 0x1
	SETTINGS_ENABLE_PUSH            SettingsParameterType = 0x2
	SETTINGS_MAX_CONCURRENT_STREAMS SettingsParameterType = 0x3
	SETTINGS_INITIAL_WINDOW_SIZE    SettingsParameterType = 0x4
	SETTINGS_MAX_FRAME_SIZE         SettingsParameterType = 0x5
	SETTINGS_MAX_HEADER_LIST_SIZE   SettingsParameterType = 0x6
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
	GetHeader() *FrameHeader
}

type FrameBase struct {
	Header FrameHeader
}

func (f *FrameBase) GetHeader() *FrameHeader {
	return &f.Header
}

type DataFrame struct {
	FrameBase
	Payload DataPayload
}

type HeadersFrame struct {
	FrameBase
	Payload HeadersPayload
}

type PriorityPayload struct {
	E                byte
	StreamDependency uint32
	Weight           byte
}

func (p *PriorityPayload) Serialize() []byte {
	var output [5]byte

	binary.BigEndian.PutUint32(output[0:4], p.StreamDependency)
	output[0] = output[0]&0x7f | ((p.E & 0x01) << 7)

	output[4] = p.Weight

	return output[:]
}

func (p *PriorityPayload) Deserialize(input []byte) error {
	var tmp uint32
	tmp = binary.BigEndian.Uint32(input[0:4])

	p.E = byte((tmp >> 31) & 0x01)
	p.StreamDependency = tmp & 0x7fffffff

	p.Weight = input[4]

	return nil

}

func (p *PriorityPayload) Size() int {
	return 5
}

type PriorityFrame struct {
	FrameBase
	Payload PriorityPayload
}

type RstStreamPayload struct {
	ErrorCode uint32
}

func (p *RstStreamPayload) Serialize() []byte {
	var output [4]byte
	binary.BigEndian.PutUint32(output[0:4], p.ErrorCode)
	return output[:]
}

func (p *RstStreamPayload) Deserialize(input []byte) error {
	p.ErrorCode = binary.BigEndian.Uint32(input[0:4])
	return nil
}

func (p *RstStreamPayload) Size() int {
	return 4
}

type RstStreamFrame struct {
	FrameBase
	Payload RstStreamPayload
}

type SettingsFrame struct {
	FrameBase
	Payload SettingsPayload
}

type PushPromisePayload struct {
	PromisedStreamID    uint32
	HeaderBlockFragment []byte
}

func (p *PushPromisePayload) Serialize() []byte {
	output := make([]byte, len(p.HeaderBlockFragment)+4)

	binary.BigEndian.PutUint32(output[0:4], p.PromisedStreamID)
	copy(output[4:], p.HeaderBlockFragment)
	return output
}

func (p *PushPromisePayload) Deserialize(input []byte, padded bool) error {
	i := 0
	var padLength byte = 0
	if padded {
		padLength = input[i]
		i++
	}

	p.PromisedStreamID = binary.BigEndian.Uint32(input[i:i+4]) & 0x7fffffff
	p.HeaderBlockFragment = input[i : len(input)-i-int(padLength)]
	return nil
}

type PushPromiseFrame struct {
	FrameBase
	Payload PushPromisePayload
}

type PingPayload struct {
	OpaqueData [8]byte
}

func (p *PingPayload) Serialize() []byte {
	output := make([]byte, len(p.OpaqueData))
	copy(output[0:], p.OpaqueData[:])
	return output
}

func (p *PingPayload) Deserialize(input []byte) error {
	copy(p.OpaqueData[:], input[0:len(p.OpaqueData)])
	return nil
}

func (p *PingPayload) Size() int {
	return 8
}

type PingFrame struct {
	FrameBase
	Payload PingPayload
}

type GoawayFrame struct {
	FrameBase
	Payload GoawayPayload
}

type WindowUpdateFrame struct {
	FrameBase
	Payload WindowUpdatePayload
}

type ContinuationPayload struct {
	HeaderBlockFragment []byte
}

func (p *ContinuationPayload) Serialize() []byte {
	output := make([]byte, len(p.HeaderBlockFragment))
	copy(output[4:], p.HeaderBlockFragment)
	return output
}

func (p *ContinuationPayload) Deserialize(input []byte) error {
	p.HeaderBlockFragment = input[:]
	return nil
}

type ContinuationFrame struct {
	FrameBase
	Payload ContinuationPayload
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

	buf := make([]byte, header.Length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return nil, err
	}

	switch header.Type {
	case DATA:
		var frame DataFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case HEADERS:
		var frame HeadersFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf, header.Flags.Has(PADDED), header.Flags.Has(FLAGS_PRIORITY)); err != nil {
			return nil, err
		}
		return &frame, nil

	case PRIORITY:
		var frame PriorityFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case RST_STREAM:
		var frame RstStreamFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case SETTINGS:
		var frame SettingsFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case PUSH_PROMISE:
		var frame PushPromiseFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf, header.Flags.Has(PADDED)); err != nil {
			return nil, err
		}
		return &frame, nil

	case PING:
		var frame PingFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case GOAWAY:
		var frame GoawayFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case WINDOW_UPDATE:
		var frame WindowUpdateFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	case CONTINUATION:
		var frame ContinuationFrame
		frame.Header = header

		if err := frame.Payload.Deserialize(buf); err != nil {
			return nil, err
		}
		return &frame, nil

	default:
		var frame UnknownFrame
		frame.Header = header
		frame.Payload = buf

		return &frame, nil
	}

}
