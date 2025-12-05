package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/http2"
)

type Stats struct {
	totalRequests   atomic.Int64
	successRequests atomic.Int64
	failedRequests  atomic.Int64
	totalLatency    atomic.Int64 // in microseconds
	minLatency      atomic.Int64
	maxLatency      atomic.Int64
}

func (s *Stats) recordRequest(latency time.Duration, success bool) {
	s.totalRequests.Add(1)
	if success {
		s.successRequests.Add(1)
	} else {
		s.failedRequests.Add(1)
	}

	latencyMicros := latency.Microseconds()
	s.totalLatency.Add(latencyMicros)

	// Update min latency
	for {
		current := s.minLatency.Load()
		if current == 0 || latencyMicros < current {
			if s.minLatency.CompareAndSwap(current, latencyMicros) {
				break
			}
		} else {
			break
		}
	}

	// Update max latency
	for {
		current := s.maxLatency.Load()
		if latencyMicros > current {
			if s.maxLatency.CompareAndSwap(current, latencyMicros) {
				break
			}
		} else {
			break
		}
	}
}

func (s *Stats) print(duration time.Duration) {
	total := s.totalRequests.Load()
	success := s.successRequests.Load()
	failed := s.failedRequests.Load()
	totalLatency := s.totalLatency.Load()

	fmt.Println("\n=== Performance Test Results ===")
	fmt.Printf("Duration:        %v\n", duration)
	fmt.Printf("Total Requests:  %d\n", total)
	fmt.Printf("Successful:      %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("Failed:          %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)
	fmt.Printf("Requests/sec:    %.2f\n", float64(total)/duration.Seconds())
	fmt.Println()
	fmt.Printf("Latency Stats:\n")
	if success > 0 {
		fmt.Printf("  Min:     %.2f ms\n", float64(s.minLatency.Load())/1000)
		fmt.Printf("  Max:     %.2f ms\n", float64(s.maxLatency.Load())/1000)
		fmt.Printf("  Average: %.2f ms\n", float64(totalLatency)/float64(success)/1000)
	}
}

func main() {
	gatewayURL := flag.String("url", os.Getenv("GATEWAY_URL"), "Gateway URL")
	host := flag.String("host", "api.example.com", "Host header")
	path := flag.String("path", "/api/test", "Request path")
	concurrency := flag.Int("c", 10, "Number of concurrent workers")
	requests := flag.Int("n", 1000, "Total number of requests")
	duration := flag.Duration("d", 0, "Test duration (overrides -n)")
	http2 := flag.Bool("http2", false, "Use HTTP/2")
	flag.Parse()

	if *gatewayURL == "" {
		*gatewayURL = "http://localhost:8080"
	}

	fullURL := *gatewayURL + *path

	fmt.Println("=== HTTP Gateway Performance Test ===")
	fmt.Printf("URL:         %s\n", fullURL)
	fmt.Printf("Host:        %s\n", *host)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	if *duration > 0 {
		fmt.Printf("Duration:    %v\n", *duration)
	} else {
		fmt.Printf("Requests:    %d\n", *requests)
	}
	fmt.Printf("HTTP/2:      %v\n", *http2)
	fmt.Println()

	stats := &Stats{}
	var wg sync.WaitGroup

	startTime := time.Now()
	stopChan := make(chan struct{})

	// Start duration timer if specified
	if *duration > 0 {
		go func() {
			time.Sleep(*duration)
			close(stopChan)
		}()
	}

	// Create HTTP client
	transport := &http.Transport{
		MaxIdleConns:        *concurrency,
		MaxIdleConnsPerHost: *concurrency,
		IdleConnTimeout:     90 * time.Second,
	}

	if *http2 {
		// Enable HTTP/2 support
		http2.ConfigureTransport(transport)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	// Progress reporting
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastCount := int64(0)
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				current := stats.totalRequests.Load()
				rps := current - lastCount
				lastCount = current
				fmt.Printf("\rProgress: %d requests (%.0f req/s)", current, float64(rps))
			}
		}
	}()

	// Start workers
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			if *duration > 0 {
				// Duration-based test
				for {
					select {
					case <-stopChan:
						return
					default:
						makeRequest(client, fullURL, *host, stats)
					}
				}
			} else {
				// Count-based test
				requestsPerWorker := *requests / *concurrency
				if workerID == 0 {
					requestsPerWorker += *requests % *concurrency // First worker gets remainder
				}

				for j := 0; j < requestsPerWorker; j++ {
					makeRequest(client, fullURL, *host, stats)
				}
			}
		}(i)
	}

	wg.Wait()
	if *duration > 0 {
		select {
		case <-stopChan:
		default:
			close(stopChan)
		}
	}

	totalDuration := time.Since(startTime)
	fmt.Println() // New line after progress

	stats.print(totalDuration)

	// Exit with error if there were failures
	if stats.failedRequests.Load() > 0 {
		os.Exit(1)
	}
}

func makeRequest(client *http.Client, url, host string, stats *Stats) {
	start := time.Now()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		stats.recordRequest(time.Since(start), false)
		return
	}
	req.Host = host

	resp, err := client.Do(req)
	if err != nil {
		stats.recordRequest(time.Since(start), false)
		return
	}

	// Read and discard body
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	stats.recordRequest(time.Since(start), success)
}
