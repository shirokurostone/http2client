package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"log"
	"net"
)

type Connection struct {
	Streams      map[uint32]Stream
	Conn         *net.Conn
	Tls          *tls.Conn
	Reader       *bufio.Reader
	Writer       *bufio.Writer
	scheme       string
	nextStreamID uint32
}

func Dial(address string) (*Connection, error) {
	var c Connection
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	c.Streams = make(map[uint32]Stream)
	c.Conn = &conn
	c.Reader = bufio.NewReader(conn)
	c.Writer = bufio.NewWriter(conn)
	c.scheme = "http"
	c.nextStreamID = 1
	return &c, nil
}

func DialTls(address string) (*Connection, error) {
	var c Connection
	conn, err := tls.Dial("tcp", address, &tls.Config{
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})

	if err != nil {
		return nil, err
	}
	c.Streams = make(map[uint32]Stream)
	c.Tls = conn
	c.Reader = bufio.NewReader(conn)
	c.Writer = bufio.NewWriter(conn)
	c.scheme = "https"
	c.nextStreamID = 1
	return &c, nil
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
			fmt.Printf("Recv: %#v\n", frame)
			header := frame.GetHeader()

			if stream, ok := c.Streams[header.StreamIdentifier]; ok {
				stream.recv <- frame
			}
		}

	}(c)

	header := FrameHeader{
		Length:           0,
		Type:             SETTINGS,
		Flags:            0,
		StreamIdentifier: 0,
	}

	fmt.Printf("Send: %#v\n", header)
	c.Writer.Write(header.Serialize())
	c.Writer.Flush()

	<-c.Streams[0].recv
	<-c.Streams[0].recv

	header = FrameHeader{
		Length:           0,
		Type:             SETTINGS,
		Flags:            ACK,
		StreamIdentifier: 0,
	}

	fmt.Printf("Send: %#v\n", header)
	c.Writer.Write(header.Serialize())
	c.Writer.Flush()

	<-c.Streams[0].recv
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

	hp := HeadersPayload{
		PadLength:           0,
		E:                   0,
		StreamDependency:    0,
		Weight:              0,
		HeaderBlockFragment: hl,
	}
	hpData := hp.Serialize()

	header := FrameHeader{
		Length:           uint32(len(hpData)),
		Type:             HEADERS,
		Flags:            END_STREAM | END_HEADERS,
		StreamIdentifier: sid,
	}

	fmt.Printf("Send: %#v %#v\n", header, hpData)
	c.Writer.Write(header.Serialize())
	c.Writer.Write(hpData)
	c.Writer.Flush()

	response := Response{}

	if f, ok := (<-c.Streams[sid].recv).(*HeadersFrame); ok {
		hl := ParseHeaderField(f.Payload.HeaderBlockFragment)

		response.Header = make(map[string][]string)
		for i := 0; i < len(hl); i++ {
			if hl[i].representationType != DYNAMIC_TABLE_SIZE_UPDATE {
				if v, ok := response.Header[hl[i].Name]; ok {
					response.Header[hl[i].Name] = append(v, hl[i].Value)
				} else {
					response.Header[hl[i].Name] = []string{hl[i].Value}
				}
			}
		}
		fmt.Printf("Headers: %#v\n", hl)
	}

	if d, ok := (<-c.Streams[sid].recv).(*DataFrame); ok {
		response.Body = string(d.Payload.Data)
		fmt.Printf("Body:\n%s", response.Body)
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
