package main

import (
	"encoding/binary"
	"io"
)

const HTTP2CoccectionPreface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

type FrameType uint8

const (
	FrameTypeData         FrameType = 0x00
	FrameTypeHeaders      FrameType = 0x01
	FrameTypePriority     FrameType = 0x02
	FrameTypeRstStream    FrameType = 0x03
	FrameTypeSettings     FrameType = 0x04
	FrameTypePushPromise  FrameType = 0x05
	FrameTypePing         FrameType = 0x06
	FrameTypeGoaway       FrameType = 0x07
	FrameTypeWindowUpdate FrameType = 0x08
	FrameTypeContinuation FrameType = 0x09
)

type Flags uint8

const (
	FlagsAck           Flags = 0x01
	FlagsEndStream     Flags = 0x01
	FlagsEndHeaders    Flags = 0x04
	FlagsPadded        Flags = 0x08
	FlagsFlagsPriority Flags = 0x20
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

type Frame interface {
	Serialize() []byte
	Deserialize(header []byte, payload []byte) error
	GetHeader() *FrameHeader
}

type FrameBase struct {
	Header FrameHeader
}

func (f *FrameBase) GetHeader() *FrameHeader {
	return &f.Header
}

////////////////////////////////////////////////////////////

type DataFrame struct {
	FrameBase
	Payload DataPayload
}

func (frame *DataFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *DataFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

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

////////////////////////////////////////////////////////////

type HeadersFrame struct {
	FrameBase
	Payload HeadersPayload
}

func (frame *HeadersFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *HeadersFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload, frame.Header.Flags.Has(FlagsPadded), frame.Header.Flags.Has(FlagsFlagsPriority)); err != nil {
		return err
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

////////////////////////////////////////////////////////////

type PriorityFrame struct {
	FrameBase
	Payload PriorityPayload
}

func (frame *PriorityFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *PriorityFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
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

////////////////////////////////////////////////////////////

type RstStreamFrame struct {
	FrameBase
	Payload RstStreamPayload
}

func (frame *RstStreamFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *RstStreamFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
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

////////////////////////////////////////////////////////////

type SettingsFrame struct {
	FrameBase
	Payload SettingsPayload
}

func (frame *SettingsFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *SettingsFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
}

type SettingsParameter struct {
	Identifier SettingsParameterType
	Value      uint32
}

type SettingsPayload struct {
	Parameters []SettingsParameter
}

func (p *SettingsParameter) Serialize() []byte {
	var output [6]byte
	var tmp [4]byte

	binary.BigEndian.PutUint16(tmp[0:2], uint16(p.Identifier))
	copy(output[0:2], tmp[0:2])
	binary.BigEndian.PutUint32(tmp[0:4], p.Value)
	copy(output[2:6], tmp[0:4])

	return output[:]
}

func (p *SettingsParameter) Deserialize(input []byte) error {
	p.Identifier = SettingsParameterType(binary.BigEndian.Uint16(input[0:2]))
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

////////////////////////////////////////////////////////////

type PushPromiseFrame struct {
	FrameBase
	Payload PushPromisePayload
}

func (frame *PushPromiseFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *PushPromiseFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload, frame.Header.Flags.Has(FlagsPadded)); err != nil {
		return err
	}

	return nil
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

////////////////////////////////////////////////////////////

type PingFrame struct {
	FrameBase
	Payload PingPayload
}

func (frame *PingFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *PingFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
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

////////////////////////////////////////////////////////////

type GoawayFrame struct {
	FrameBase
	Payload GoawayPayload
}

func (frame *GoawayFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *GoawayFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
}

type GoawayPayload struct {
	LastStreamID        uint32
	ErrorCode           uint32
	AdditionalDebugData []byte
}

func (p *GoawayPayload) Serialize() []byte {
	var output [8]byte
	binary.BigEndian.PutUint32(output[0:4], p.LastStreamID&0x7fffffff)
	binary.BigEndian.PutUint32(output[4:8], p.ErrorCode)

	return append(output[:], p.AdditionalDebugData...)
}

func (p *GoawayPayload) Deserialize(input []byte) error {

	tmp := binary.BigEndian.Uint32(input[0:4])
	p.LastStreamID = tmp & 0x7fffffff

	tmp = binary.BigEndian.Uint32(input[4:8])
	p.ErrorCode = tmp

	p.AdditionalDebugData = input[8:]

	return nil
}

////////////////////////////////////////////////////////////

type WindowUpdateFrame struct {
	FrameBase
	Payload WindowUpdatePayload
}

func (frame *WindowUpdateFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *WindowUpdateFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
}

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

////////////////////////////////////////////////////////////

type ContinuationFrame struct {
	FrameBase
	Payload ContinuationPayload
}

func (frame *ContinuationFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Serialize()...)
}

func (frame *ContinuationFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	if err := frame.Payload.Deserialize(payload); err != nil {
		return err
	}

	return nil
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

////////////////////////////////////////////////////////////

type UnknownFrame struct {
	FrameBase
	Payload UnknownPayload
}

func (frame *UnknownFrame) Serialize() []byte {
	header := frame.Header.Serialize()

	return append(header, frame.Payload.Payload...)
}

func (frame *UnknownFrame) Deserialize(header []byte, payload []byte) error {
	if err := frame.Header.Deserialize(header); err != nil {
		return err
	}

	frame.Payload.Payload = payload

	return nil
}

type UnknownPayload struct {
	Payload []byte
}

////////////////////////////////////////////////////////////

type ErrorCode uint32

const (
	ErrorCodeNoError            ErrorCode = 0x00
	ErrorCodeProtocolError      ErrorCode = 0x01
	ErrorCodeInternalError      ErrorCode = 0x02
	ErrorCodeFlowControlError   ErrorCode = 0x03
	ErrorCodeSettingsTimeout    ErrorCode = 0x04
	ErrorCodeStreamClosed       ErrorCode = 0x05
	ErrorCodeFrameSizeError     ErrorCode = 0x06
	ErrorCodeRefusedStream      ErrorCode = 0x07
	ErrorCodeCancel             ErrorCode = 0x08
	ErrorCodeCompressionError   ErrorCode = 0x09
	ErrorCodeConnectError       ErrorCode = 0x0a
	ErrorCodeEnhanceYourCalm    ErrorCode = 0x0b
	ErrorCodeInadequateSecurity ErrorCode = 0x0c
	ErrorCodeHTTP11Required     ErrorCode = 0x0d
)

type SettingsParameterType uint16

const (
	SettingsHeaderTableSize      SettingsParameterType = 0x1
	SettingsEnablePush           SettingsParameterType = 0x2
	SettingsMaxConcurrentStreams SettingsParameterType = 0x3
	SettingsInitialWindowSize    SettingsParameterType = 0x4
	SettingsMaxFrameSize         SettingsParameterType = 0x5
	SettingsMaxHeaderListSize    SettingsParameterType = 0x6
)

func ReadFrame(reader io.Reader) (Frame, error) {
	var header FrameHeader

	headerBytes := make([]byte, header.Size())
	_, err := io.ReadFull(reader, headerBytes[:])
	if err != nil {
		return nil, err
	}

	err = header.Deserialize(headerBytes[:])
	if err != nil {
		return nil, err
	}

	payloadBytes := make([]byte, header.Length)
	if _, err := io.ReadFull(reader, payloadBytes); err != nil {
		return nil, err
	}

	frame := newBlankFrame(header.Type)
	if err := frame.Deserialize(headerBytes, payloadBytes); err != nil {
		return nil, err
	}
	return frame, nil
}

func newBlankFrame(t FrameType) Frame {
	switch t {
	case FrameTypeData:
		return &DataFrame{}
	case FrameTypeHeaders:
		return &HeadersFrame{}
	case FrameTypePriority:
		return &PriorityFrame{}
	case FrameTypeRstStream:
		return &RstStreamFrame{}
	case FrameTypeSettings:
		return &SettingsFrame{}
	case FrameTypePushPromise:
		return &PushPromiseFrame{}
	case FrameTypePing:
		return &PingFrame{}
	case FrameTypeGoaway:
		return &GoawayFrame{}
	case FrameTypeWindowUpdate:
		return &WindowUpdateFrame{}
	case FrameTypeContinuation:
		return &ContinuationFrame{}
	default:
		return &UnknownFrame{}
	}
}
