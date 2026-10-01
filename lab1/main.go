package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
)

func main() {
	http.HandleFunc("/health", HealthCheck())
	http.HandleFunc("/eat", EatRam())
	http.HandleFunc("/burn", BurnCpu())

	fmt.Println("Starting server")
	if e := http.ListenAndServe(":8080", nil); e != nil {
		fmt.Println("Error starting server:", e)
		slog.Error("Error starting server")
		panic(e)
	}
	fmt.Println("started server")
}

func HealthCheck() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

var (
	mu     sync.Mutex
	holded [][]byte
)

func EatRam() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ns := r.URL.Query().Get("mb")
		n, err := strconv.Atoi(ns)
		if err != nil || n <= 0 || n > 1024 {
			http.Error(w, "n must be 1..1024", http.StatusBadRequest)
			return
		}
		buf := make([]byte, n*1024*1024)
		for i := 0; i < len(buf); i += 4096 {
			buf[i] = 1
		}

		mu.Lock()
		holded = append(holded, buf)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
}

func BurnCpu() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for range 100000000000 {
			i := 1000 * 1000
			_ = i
		}
	})
}
