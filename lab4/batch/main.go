package main

import (
	"context"
	"crypto/sha256"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func positiveEnv(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		log.Fatalf("%s must be a positive integer, got %q", name, raw)
	}
	return value
}

func main() {
	memoryMiB := positiveEnv("MEMORY_MIB", 64)
	workers := positiveEnv("CPU_WORKERS", 1)
	if memoryMiB > int(^uint(0)>>1)/(1024*1024) {
		log.Fatal("MEMORY_MIB overflows allocation size")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Touch each page so the allocation becomes resident, not just virtual.
	memory := make([]byte, memoryMiB*1024*1024)
	touch := func() {
		for i := 0; i < len(memory); i += os.Getpagesize() {
			memory[i]++
		}
	}
	touch()
	log.Printf("batch started: pid=%d memory_mib=%d cpu_workers=%d", os.Getpid(), memoryMiB, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(seed byte) {
			defer wg.Done()
			data := [32]byte{seed}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for j := 0; j < 4096; j++ {
						data = sha256.Sum256(data[:])
					}
				}
			}
		}(byte(i))
	}

	// Keep allocated pages active throughout the experiment.
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			runtime.KeepAlive(memory)
			log.Print("batch stopped")
			return
		case <-ticker.C:
			touch()
		}
	}
}
