package main

import (
	"io"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		io.WriteString(w, "OK")
	})

	http.ListenAndServeTLS(":8443", "server.crt", "private.key", nil)
}
