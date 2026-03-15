package main

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"
)

var (
	// Счетчики HTTP статусов (атомарные)
	status1xx   int64
	status2xx   int64
	status3xx   int64
	status4xx   int64
	status5xx   int64
	statusOther int64

	// Счетчики сетевых ошибок
	timeoutErrors int64
	resetErrors   int64
	eofErrors     int64

	// Общий счетчик запросов
	totalRequests int64
)

func incrementCounter(counter *int64) {
	atomic.AddInt64(counter, 1)
}

func resetCounters() {
	atomic.StoreInt64(&status1xx, 0)
	atomic.StoreInt64(&status2xx, 0)
	atomic.StoreInt64(&status3xx, 0)
	atomic.StoreInt64(&status4xx, 0)
	atomic.StoreInt64(&status5xx, 0)
	atomic.StoreInt64(&statusOther, 0)
	atomic.StoreInt64(&timeoutErrors, 0)
	atomic.StoreInt64(&resetErrors, 0)
	atomic.StoreInt64(&eofErrors, 0)
	atomic.StoreInt64(&totalRequests, 0)

	fmt.Println("\n" + strings.Repeat("-", 50))
	fmt.Println("СЧЕТЧИКИ СБРОШЕНЫ")
	fmt.Println(strings.Repeat("-", 50) + "\n")
}

func printStats() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Атомарное чтение всех счётчиков
		total := atomic.LoadInt64(&totalRequests)
		s1xx := atomic.LoadInt64(&status1xx)
		s2xx := atomic.LoadInt64(&status2xx)
		s3xx := atomic.LoadInt64(&status3xx)
		s4xx := atomic.LoadInt64(&status4xx)
		s5xx := atomic.LoadInt64(&status5xx)
		sOther := atomic.LoadInt64(&statusOther)
		tErrors := atomic.LoadInt64(&timeoutErrors)
		rErrors := atomic.LoadInt64(&resetErrors)
		eErrors := atomic.LoadInt64(&eofErrors)

		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("СТАТИСТИКА СЕРВЕРА (обновление каждые 5 сек)")
		fmt.Println(strings.Repeat("=", 60))

		fmt.Printf("Всего запросов: %d\n", total)

		fmt.Println("\nHTTP СТАТУСЫ:")
		fmt.Printf("  1xx: %d\n", s1xx)
		fmt.Printf("  2xx: %d\n", s2xx)
		fmt.Printf("  3xx: %d\n", s3xx)
		fmt.Printf("  4xx: %d\n", s4xx)
		fmt.Printf("  5xx: %d\n", s5xx)
		fmt.Printf("  Other: %d\n", sOther)

		fmt.Println("\nСЕТЕВЫЕ ОШИБКИ (REAL):")
		fmt.Printf("  Connection Reset: %d\n", rErrors)
		fmt.Printf("  Premature EOF: %d\n", eErrors)
		fmt.Printf("  Timeouts (slow): %d\n", tErrors)

		if total > 0 {
			totalFloat := float64(total)
			fmt.Println("\nПРОЦЕНТНОЕ СООТНОШЕНИЕ:")
			fmt.Printf("  2xx: %.2f%%\n", float64(s2xx)/totalFloat*100)
			fmt.Printf("  4xx: %.2f%%\n", float64(s4xx)/totalFloat*100)
			fmt.Printf("  5xx: %.2f%%\n", float64(s5xx)/totalFloat*100)
			fmt.Printf("  Connection Reset: %.2f%%\n", float64(rErrors)/totalFloat*100)
			fmt.Printf("  Premature EOF: %.2f%%\n", float64(eErrors)/totalFloat*100)
			fmt.Printf("  Timeouts: %.2f%%\n", float64(tErrors)/totalFloat*100)
		}

		fmt.Println(strings.Repeat("=", 60) + "\n")
	}
}

func handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	fmt.Println("\nУПРАВЛЕНИЕ:")
	fmt.Println("  - Ctrl+C для выхода")
	fmt.Println("  - Введите 'reset' для сброса счетчиков")

	go func() {
		var input string
		for {
			fmt.Scanln(&input)
			if input == "reset" {
				resetCounters()
			}
		}
	}()

	<-sigChan
	fmt.Println("\nЗавершение работы...")
	os.Exit(0)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	go handleSignals()
	go printStats()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		incrementCounter(&totalRequests)

		randomNum := rand.Intn(100)

		switch {
		// REAL NETWORK ERROR 1: Connection Reset (4%)
		case randomNum < 4:
			incrementCounter(&resetErrors)
			fmt.Printf("[RESET] %s - Closing with RST packet\n", r.RemoteAddr)
			
			// Получаем TCP-соединение
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			
			// Устанавливаем SO_LINGER с таймаутом 0, чтобы вызвать RST
			tcpConn, ok := conn.(*net.TCPConn)
			if ok {
				tcpConn.SetLinger(0)
			}
			conn.Close()
			return

		// REAL NETWORK ERROR 2: Premature EOF (4%)
		case randomNum < 8:
			incrementCounter(&eofErrors)
			fmt.Printf("[EOF] %s - Closing without response\n", r.RemoteAddr)
			
			// Просто закрываем соединение без ответа
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				return
			}
			conn.Close()
			return

		// Таймаут (имитация работы) - 6%
		case randomNum < 14:
			incrementCounter(&timeoutErrors)
			fmt.Printf("[TIMEOUT] %s - Sleeping 2 seconds\n", r.RemoteAddr)
			time.Sleep(2 * time.Second)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "200 OK (after timeout)")

		// 1xx - 2%
		case randomNum < 16:
			incrementCounter(&status1xx)
			w.WriteHeader(http.StatusContinue)
			fmt.Fprintf(w, "100 Continue")

		// Other (418) - 5%
		case randomNum < 21:
			incrementCounter(&statusOther)
			w.WriteHeader(http.StatusTeapot)
			fmt.Fprintf(w, "418 I'm a teapot")

		// 3xx - 6%
		case randomNum < 27:
			incrementCounter(&status3xx)
			w.Header().Set("Location", "/new-location")
			w.WriteHeader(http.StatusMovedPermanently)
			fmt.Fprintf(w, "301 Moved Permanently")

		// 5xx - 8%
		case randomNum < 35:
			incrementCounter(&status5xx)
			switch rand.Intn(2) {
			case 0:
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "500 Internal Server Error")
			case 1:
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, "503 Service Unavailable")
			}

		// 4xx - 10%
		case randomNum < 45:
			incrementCounter(&status4xx)
			switch rand.Intn(3) {
			case 0:
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "400 Bad Request")
			case 1:
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprintf(w, "401 Unauthorized")
			case 2:
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, "404 Not Found")
			}

		// 2xx - 55%
		default:
			incrementCounter(&status2xx)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "200 OK")
		}
	})

	fmt.Println("\n" + strings.Repeat("-", 40))
	fmt.Println("СЕРВЕР ЗАПУЩЕН НА :8000")
	fmt.Println(strings.Repeat("-", 40))
	fmt.Println("\nРАСПРЕДЕЛЕНИЕ (С РЕАЛЬНЫМИ СЕТЕВЫМИ ОШИБКАМИ):")
	fmt.Println("  4% - Connection Reset (RST packet) -> err != nil")
	fmt.Println("  4% - Premature EOF (close without response) -> err != nil")
	fmt.Println("  6% - Таймаут (2 сек, потом 200 OK)")
	fmt.Println("  2% - 1xx")
	fmt.Println("  5% - Other (418)")
	fmt.Println("  6% - 3xx")
	fmt.Println("  8% - 5xx")
	fmt.Println("  10% - 4xx")
	fmt.Println("  55% - 2xx")
	fmt.Println("\nСумма: 100%")
	fmt.Println("\nСтатистика каждые 5 сек")
	fmt.Println("Введите 'reset' для сброса счетчиков\n")

	if err := http.ListenAndServe(":8000", handler); err != nil {
		fmt.Printf("Ошибка сервера: %v\n", err)
	}
}