package main

import (
	"fmt"
	"log"
)

func main() {
	conn, err := DialTls(":8443")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	conn.StartHTTP2()

	resp, err := conn.Request("GET", "/", []HeaderField{
		{"host", "localhost"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%#v\n", resp)
}
