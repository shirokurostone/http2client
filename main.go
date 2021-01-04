package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"log"
)

func recvFrame(reader io.Reader) Frame {
	frame, err := ReadFrame(reader)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Recv: %#v\n", frame)
	return frame
}

func main() {
	conn, err := tls.Dial("tcp", ":8443", &tls.Config{
		NextProtos:         []string{"h2"},
		InsecureSkipVerify: true,
	})
	// conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	writer.Write([]byte(HTTP2CoccectionPreface))

	header := FrameHeader{
		Length:           0,
		Type:             SETTINGS,
		Flags:            0,
		StreamIdentifier: 0,
	}

	fmt.Printf("Send: %#v\n", header)
	writer.Write(header.Serialize())
	writer.Flush()

	recvFrame(reader)
	recvFrame(reader)

	header = FrameHeader{
		Length:           0,
		Type:             SETTINGS,
		Flags:            1,
		StreamIdentifier: 0,
	}

	fmt.Printf("Send: %#v\n", header)
	writer.Write(header.Serialize())
	writer.Flush()

	recvFrame(reader)

	headers := []HeaderField{
		{":method", "GET"},
		{":scheme", "http"},
		{":path", "/"},
		{"host", "localhost"},
	}

	hl, err := EncodeHeaders(headers)
	if err != nil {
		log.Fatal(err)
	}

	hp := HeadersPayload{
		PadLength:           0,
		E:                   0,
		StreamDependency:    0,
		Weight:              0,
		HeaderBlockFragment: hl,
	}
	hpData := hp.Serialize()

	header = FrameHeader{
		Length:           uint32(len(hpData)),
		Type:             HEADERS,
		Flags:            END_STREAM | END_HEADERS,
		StreamIdentifier: 1,
	}

	fmt.Printf("Send: %#v %#v\n", header, hpData)
	writer.Write(header.Serialize())
	writer.Write(hpData)
	writer.Flush()

	if f, ok := recvFrame(reader).(*HeadersFrame); ok {
		hl := ParseHeaderField(f.Payload.HeaderBlockFragment)
		fmt.Printf("Headers: %#v\n", hl)
	}

	if d, ok := recvFrame(reader).(*DataFrame); ok {
		fmt.Printf("Body:\n%s", string(d.Payload.Data))
	}

}
