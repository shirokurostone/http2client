package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"math"
	"net"
)

type Connection struct {
	Streams       map[uint32]Stream
	Conn          *net.Conn
	Tls           *tls.Conn
	Reader        *bufio.Reader
	Writer        *bufio.Writer
	scheme        string
	nextStreamID  uint32
	HeaderDecoder HeaderDecoder

	EnablePush           bool
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	MaxHeaderListSize    uint32

	Window uint32
}

func newConnection(conn *net.Conn, tls *tls.Conn, reader *bufio.Reader, writer *bufio.Writer, scheme string) (*Connection, error) {
	var c Connection
	c.Streams = make(map[uint32]Stream)
	c.Conn = conn
	c.Tls = tls
	c.Reader = reader
	c.Writer = writer
	c.scheme = scheme
	c.nextStreamID = 1
	c.HeaderDecoder = HeaderDecoder{
		DynamicTable: []HeaderField{},
		MaxSize:      4096,
	}
	c.EnablePush = true
	c.MaxConcurrentStreams = math.MaxUint32
	c.InitialWindowSize = 65535
	c.MaxFrameSize = 16384
	c.MaxHeaderListSize = math.MaxUint32

	c.Window = c.InitialWindowSize
	return &c, nil
}

func Dial(address string) (*Connection, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return newConnection(&conn, nil, bufio.NewReader(conn), bufio.NewWriter(conn), "http")
}

func DialTls(address string) (*Connection, error) {
	conn, err := tls.Dial("tcp", address, &tls.Config{
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil, err
	}
	return newConnection(nil, conn, bufio.NewReader(conn), bufio.NewWriter(conn), "https")
}

func (c *Connection) Close() {
	if c.Conn != nil {
		(*c.Conn).Close()
		c.Conn = nil
	}
	if c.Tls != nil {
		c.Tls.Close()
		c.Tls = nil
	}
}

func (c *Connection) sendFrame(frame Frame) {
	fmt.Printf("Send: %#v\n", frame)
	c.Writer.Write(frame.Serialize())
	c.Writer.Flush()
}

func (c *Connection) handleRecievedFrame(frame Frame) {
	fmt.Printf("Recv: %#v\n", frame)
	header := frame.GetHeader()

	if d, ok := frame.(*DataFrame); ok {
		c.Window -= d.Header.Length
		if c.Window <= 0 {
			wf := WindowUpdateFrame{
				FrameBase: FrameBase{
					Header: FrameHeader{
						Length:           4,
						Type:             FrameTypeWindowUpdate,
						Flags:            0,
						StreamIdentifier: 0,
					},
				},
				Payload: WindowUpdatePayload{
					WindowSizeIncrement: c.InitialWindowSize,
				},
			}
			c.sendFrame(&wf)
			c.Window += c.InitialWindowSize
		}
	}

	if stream, ok := c.Streams[header.StreamIdentifier]; ok {
		stream.recv <- frame
	}
}

func (c *Connection) handleConnectionFrame() {
	sid := uint32(0)

	frame := <-c.Streams[sid].recv
	if s, ok := frame.(*SettingsFrame); ok {
		if !s.Header.Flags.Has(FlagsAck) {

			for _, p := range s.Payload.Parameters {
				switch p.Identifier {
				case SettingsHeaderTableSize:
					c.HeaderDecoder.MaxSize = int(p.Value)
					break
				case SettingsEnablePush:
					if p.Value == 0 {
						c.EnablePush = false
					} else if p.Value == 1 {
						c.EnablePush = true
					} else {
						// error
					}
					break
				case SettingsMaxConcurrentStreams:
					c.MaxConcurrentStreams = p.Value
					break
				case SettingsInitialWindowSize:
					c.InitialWindowSize = p.Value
					break
				case SettingsMaxFrameSize:
					if 16384 <= p.Value && p.Value <= 16777215 {
						c.MaxFrameSize = p.Value
					} else {
						// error
					}
					break
				case SettingsMaxHeaderListSize:
					c.MaxHeaderListSize = p.Value
					break
				default:
					// error
				}

			}

			sf := SettingsFrame{
				FrameBase: FrameBase{
					Header: FrameHeader{
						Length:           0,
						Type:             FrameTypeSettings,
						Flags:            FlagsAck,
						StreamIdentifier: 0,
					},
				},
				Payload: SettingsPayload{
					Parameters: []SettingsParameter{},
				},
			}
			c.sendFrame(&sf)
		}
	}
}

func (c *Connection) StartHTTP2() {
	c.Writer.Write([]byte(HTTP2CoccectionPreface))

	s := Stream{
		StreamID: 0,
		State:    idle,
		recv:     make(chan Frame, 1),
	}
	c.Streams[0] = s

	go func(c *Connection) {
		for {
			frame, err := ReadFrame(c.Reader)
			if err != nil {
				log.Fatal(err)
			}
			c.handleRecievedFrame(frame)
		}
	}(c)

	sf1 := SettingsFrame{
		FrameBase: FrameBase{
			Header: FrameHeader{
				Length:           0,
				Type:             FrameTypeSettings,
				Flags:            0,
				StreamIdentifier: 0,
			},
		},
		Payload: SettingsPayload{
			Parameters: []SettingsParameter{},
		},
	}

	c.sendFrame(&sf1)

	go func(c *Connection) {
		for {
			c.handleConnectionFrame()
		}
	}(c)

}

type Response struct {
	Header map[string][]string
	Body   string
}

func (c *Connection) Request(method string, requestPath string, headers []HeaderField) (*Response, error) {

	sid := c.nextStreamID
	c.nextStreamID += 2
	s := Stream{
		StreamID: sid,
		State:    idle,
		recv:     make(chan Frame, 1),
	}
	c.Streams[sid] = s

	hs := append(
		[]HeaderField{
			HeaderField{":method", method},
			HeaderField{":scheme", c.scheme},
			HeaderField{":path", requestPath},
		},
		headers...,
	)

	hl, err := EncodeHeaders(hs)
	if err != nil {
		return nil, err
	}

	hf := HeadersFrame{
		FrameBase: FrameBase{
			Header: FrameHeader{
				Length:           0,
				Type:             FrameTypeHeaders,
				Flags:            FlagsEndStream | FlagsEndHeaders,
				StreamIdentifier: sid,
			},
		},
		Payload: HeadersPayload{
			PadLength:           0,
			E:                   0,
			StreamDependency:    0,
			Weight:              0,
			HeaderBlockFragment: hl,
		},
	}
	hf.Header.Length = uint32(len(hf.Payload.Serialize()))

	c.sendFrame(&hf)

	response := Response{
		Header: make(map[string][]string),
		Body:   "",
	}
	readingHeader := true
	headerBlockFragment := []byte{}
	window := c.InitialWindowSize

	for {
		frame := <-c.Streams[sid].recv

		if readingHeader {
			if f, ok := frame.(*HeadersFrame); ok {
				headerBlockFragment = append(headerBlockFragment, f.Payload.HeaderBlockFragment...)
			} else if c, ok := frame.(*ContinuationFrame); ok {
				headerBlockFragment = append(headerBlockFragment, c.Payload.HeaderBlockFragment...)
			} else {
				// frame error ?
				return nil, fmt.Errorf("invalid frame type : %d", frame.GetHeader().Type)
			}
		} else {
			if d, ok := frame.(*DataFrame); ok {
				window -= d.Header.Length
				response.Body = response.Body + string(d.Payload.Data)
			} else {
				// frame error ?
				return nil, fmt.Errorf("invalid frame type : %d", frame.GetHeader().Type)
			}
		}

		if frame.GetHeader().Flags.Has(FlagsEndHeaders) {
			readingHeader = false
			header := c.HeaderDecoder.Decode(headerBlockFragment)
			for key, value := range header {
				if _, ok := response.Header[key]; ok {
					response.Header[key] = append(response.Header[key], value...)
				} else {
					response.Header[key] = value
				}
			}
		} else if frame.GetHeader().Flags.Has(FlagsEndStream) {
			break
		} else if window <= 0 {
			wf := WindowUpdateFrame{
				FrameBase: FrameBase{
					Header: FrameHeader{
						Length:           4,
						Type:             FrameTypeWindowUpdate,
						Flags:            0,
						StreamIdentifier: sid,
					},
				},
				Payload: WindowUpdatePayload{
					WindowSizeIncrement: c.InitialWindowSize,
				},
			}
			c.sendFrame(&wf)
			window += c.InitialWindowSize
		}
	}

	return &response, nil
}

type StreamState byte

const (
	idle StreamState = iota
	reservedLocal
	reservedRemote
	open
	halfClosedRemote
	halfClosedLocal
	closed
)

type Stream struct {
	StreamID uint32
	State    StreamState
	recv     chan Frame
}
