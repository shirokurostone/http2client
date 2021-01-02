package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	f2 := FrameHeader{
		Length:           0,
		Type:             SETTINGS,
		Flags:            0,
		StreamIdentifier: 0,
	}

	writer.Write(f2.Serialize())
	writer.Flush()

	frame, err := ReadFrame(reader)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%#v\n", frame)
}
