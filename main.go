package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func appHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello, este es una webapp")
}

func main() {
	flagIP := flag.String("ip", "127.0.0.1", "ip address")
	flagPort := flag.String("port", "6060", "tcp port")
	flag.Parse()

	if ip := os.Getenv("VCAP_APP_HOST"); ip != "" {
		*flagIP = ip
	}
	if port := os.Getenv("VCAP_APP_PORT"); port != "" {
		*flagPort = port
	}
	http.HandleFunc("/hwbm", appHandler)
	log.Fatal(http.ListenAndServe(*flagIP+":"+*flagPort, nil))
}
