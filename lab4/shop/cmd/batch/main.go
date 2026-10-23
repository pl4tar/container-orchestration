package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

// envInt читает целое из окружения, иначе возвращает значение по умолчанию.
func envInt(name string, def int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v < 0 {
		return def
	}
	return v
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cpuWorkers := envInt("BATCH_CPU_WORKERS", 1) // сколько горутин крутят CPU
	memTargetMB := envInt("BATCH_MEM_MB", 64)    // сколько памяти держать в итоге
	memStepMB := envInt("BATCH_MEM_STEP_MB", 8)  // на сколько МБ расти за шаг
	stepEvery := time.Duration(max(envInt("BATCH_MEM_STEP_SECONDS", 1), 1)) * time.Second

	slog.Info("batch started",
		"cpu_workers", cpuWorkers, "mem_target_mb", memTargetMB,
		"mem_step_mb", memStepMB, "step_every", stepEvery)

	// Бесполезная «аналитика»: каждая горутина крутит CPU без остановки.
	for i := 0; i < cpuWorkers; i++ {
		go func() {
			x := 0
			for ctx.Err() == nil {
				for j := 0; j < 1_000_000; j++ {
					x = x*31 + j
				}
				runtime.Gosched()
			}
			_ = x
		}()
	}

	// Память растёт ступеньками до BATCH_MEM_MB и дальше удерживается.
	// Каждую страницу пишем, чтобы память стала резидентной (RSS), а не только виртуальной.
	var chunks [][]byte
	heldMB := 0
	ticker := time.NewTicker(stepEvery)
	defer ticker.Stop()
	report := time.NewTicker(10 * time.Second)
	defer report.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("batch stopped", "held_mb", heldMB)
			return
		case <-report.C:
			slog.Info("batch running", "held_mb", heldMB)
		case <-ticker.C:
			if heldMB >= memTargetMB {
				// Периодически трогаем всё удерживаемое, чтобы ядро не считало страницы холодными.
				for _, c := range chunks {
					for k := 0; k < len(c); k += 4096 {
						c[k]++
					}
				}
				continue
			}
			step := memStepMB
			if heldMB+step > memTargetMB {
				step = memTargetMB - heldMB
			}
			c := make([]byte, step<<20)
			for k := 0; k < len(c); k += 4096 {
				c[k] = 1
			}
			chunks = append(chunks, c)
			heldMB += step
		}
	}
}
