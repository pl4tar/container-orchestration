// api — подопытный сервис для лабы 1.
//
//	GET /health     -> "ok"
//	GET /eat?mb=N   -> выделяет N МБ памяти и держит их до завершения процесса
//	GET /burn       -> запускает бесконечный цикл, грузящий одно ядро CPU
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
)

var (
	mu    sync.Mutex
	eaten [][]byte // держим ссылки, чтобы GC не освободил память
	total int
)

func health(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintln(w, "ok")
}

func eat(w http.ResponseWriter, r *http.Request) {
	mb, err := strconv.Atoi(r.URL.Query().Get("mb"))
	if err != nil || mb <= 0 {
		http.Error(w, "usage: /eat?mb=N (N > 0)", http.StatusBadRequest)
		return
	}

	buf := make([]byte, mb<<20)
	// Пишем в каждую страницу: иначе ядро не выделит физическую память
	// (lazy allocation), и RSS / memory.current не вырастут.
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = 1
	}

	mu.Lock()
	eaten = append(eaten, buf)
	total += mb
	t := total
	mu.Unlock()

	log.Printf("eat: +%d MB, total %d MB", mb, t)
	fmt.Fprintf(w, "allocated %d MB, holding %d MB total\n", mb, t)
}

func burn(w http.ResponseWriter, _ *http.Request) {
	go func() {
		runtime.LockOSThread()
		for {
		}
	}()
	log.Printf("burn: started busy loop")
	fmt.Fprintln(w, "burning one CPU core")
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /eat", eat)
	mux.HandleFunc("GET /burn", burn)

	addr := ":" + port
	log.Printf("api listening on %s (pid %d)", addr, os.Getpid())
	log.Fatal(http.ListenAndServe(addr, mux))
}
