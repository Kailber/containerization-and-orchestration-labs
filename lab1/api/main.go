package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
)

var allocations [][]byte

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func eatHandler(w http.ResponseWriter, r *http.Request) {
	mbParam := r.URL.Query().Get("mb")

	if mbParam == "" {
		http.Error(w, "missing mb parameter", http.StatusBadRequest)
		return
	}

	mb, err := strconv.Atoi(mbParam)
	if err != nil || mb <= 0 {
		http.Error(w, "mb must be a positive integer", http.StatusBadRequest)
		return
	}

	// Выделяем память.
	size := mb * 1024 * 1024
	buf := make([]byte, size)

	// Записываем каждый мегабайт, чтобы реально использовать выделенную память.
	for i := 0; i < len(buf); i += 4096 {
		buf[i] = 1
	}

	// Сохраняем ссылку, чтобы Go не освободил память.
	allocations = append(allocations, buf)

	fmt.Fprintf(w, "allocated %d MB\n", mb)
}

func burnHandler(w http.ResponseWriter, r *http.Request) {
	// Бесконечная вычислительная нагрузка на одно ядро.
	var x uint64

	for {
		x++
		if x == 0 {
			break
		}
	}
}

func main() {
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/eat", eatHandler)
	http.HandleFunc("/burn", burnHandler)

	log.Println("api is listening on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
