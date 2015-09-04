package main

import (
	"fmt"
	"log"
	"net/http"
)

func appHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello, este es una webapp")
}

func main() {
	http.HandleFunc("/hwbm", appHandler)
	ip := ""
	port := "8080"
	log.Fatal(http.ListenAndServe(ip+":"+port, nil))
}
