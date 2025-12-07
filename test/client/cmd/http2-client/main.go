package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/net/http2"
)

type Config struct {
	URL             string
	Requests        int
	Concurrency     int
	UseHTTP2        bool
	UseH2C          bool
	InsecureSkipTLS bool
	Verbose         bool
	Timeout         time.Duration
}

type Result struct {
	StatusCode    int
	Duration      time.Duration
	Error         error
	ContentLength int64
	Protocol      string
}

func main() {
	config := parseFlags()

	log.Printf("HTTP/2 Client Test")
	log.Printf("==================")
	log.Printf("URL: %s", config.URL)
	log.Printf("Requests: %d", config.Requests)
	log.Printf("Concurrency: %d", config.Concurrency)
	log.Printf("HTTP/2: %v (H2C: %v)", config.UseHTTP2, config.UseH2C)
	log.Printf("")

	// Create HTTP client with HTTP/2 support
	client := createClient(config)

	// Run tests
	results := runTests(client, config)

	// Print results
	printResults(results, config)
}

func parseFlags() Config {
	var config Config

	flag.StringVar(&config.URL, "url", "http://localhost:8080", "Target URL")
	flag.IntVar(&config.Requests, "n", 10, "Number of requests")
	flag.IntVar(&config.Concurrency, "c", 1, "Concurrency level")
	flag.BoolVar(&config.UseHTTP2, "http2", true, "Use HTTP/2")
	flag.BoolVar(&config.UseH2C, "h2c", true, "Use H2C (HTTP/2 Cleartext)")
	flag.BoolVar(&config.InsecureSkipTLS, "insecure", true, "Skip TLS verification")
	flag.BoolVar(&config.Verbose, "v", false, "Verbose output")
	flag.DurationVar(&config.Timeout, "timeout", 30*time.Second, "Request timeout")

	flag.Parse()

	return config
}

func createClient(config Config) *http.Client {
	// Create TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipTLS,
	}

	// Create transport
	var transport http.RoundTripper

	if config.UseHTTP2 {
		if config.UseH2C {
			// HTTP/2 Cleartext (H2C)
			transport = &http2.Transport{
				// Allow HTTP URLs
				AllowHTTP: true,
				// Dial function for H2C
				// Disable TLS for H2C
				TLSClientConfig: nil,
			}
		} else {
			// HTTP/2 over TLS
			transport = &http2.Transport{
				TLSClientConfig: tlsConfig,
			}
		}
	} else {
		// HTTP/1.1
		transport = &http.Transport{
			TLSClientConfig:     tlsConfig,
			MaxIdleConns:        config.Concurrency,
			MaxIdleConnsPerHost: config.Concurrency,
			IdleConnTimeout:     90 * time.Second,
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}
}

func runTests(client *http.Client, config Config) []Result {
	results := make([]Result, config.Requests)
	var wg sync.WaitGroup

	// Create semaphore for concurrency control
	semaphore := make(chan struct{}, config.Concurrency)

	startTime := time.Now()

	for i := 0; i < config.Requests; i++ {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(index int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			results[index] = doRequest(client, config, index)
		}(i)
	}

	wg.Wait()

	totalDuration := time.Since(startTime)

	if config.Verbose {
		log.Printf("\nTotal duration: %v", totalDuration)
	}

	return results
}

func doRequest(client *http.Client, config Config, index int) Result {
	start := time.Now()

	req, err := http.NewRequest("GET", config.URL, nil)
	if err != nil {
		return Result{Error: err, Duration: time.Since(start)}
	}

	// Add headers
	req.Header.Set("User-Agent", "HTTP2-Client/1.0")
	req.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", index))

	// Make request
	resp, err := client.Do(req)
	if err != nil {
		return Result{Error: err, Duration: time.Since(start)}
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{
			StatusCode: resp.StatusCode,
			Error:      err,
			Duration:   time.Since(start),
			Protocol:   resp.Proto,
		}
	}

	result := Result{
		StatusCode:    resp.StatusCode,
		Duration:      time.Since(start),
		ContentLength: int64(len(body)),
		Protocol:      resp.Proto,
	}

	if config.Verbose {
		log.Printf("[%d] %s %d %v %s (body: %d bytes)",
			index, resp.Proto, resp.StatusCode, result.Duration, config.URL, len(body))
	}

	return result
}

func printResults(results []Result, config Config) {
	var (
		successCount int
		failCount    int
		totalBytes   int64
		totalTime    time.Duration
		minTime      time.Duration
		maxTime      time.Duration
		protocols    = make(map[string]int)
	)

	minTime = time.Hour // Initialize with large value
	maxTime = 0

	for _, result := range results {
		if result.Error != nil {
			failCount++
			if config.Verbose {
				log.Printf("Error: %v", result.Error)
			}
			continue
		}

		successCount++
		totalBytes += result.ContentLength
		totalTime += result.Duration

		if result.Duration < minTime {
			minTime = result.Duration
		}
		if result.Duration > maxTime {
			maxTime = result.Duration
		}

		protocols[result.Protocol]++
	}

	avgTime := time.Duration(0)
	if successCount > 0 {
		avgTime = totalTime / time.Duration(successCount)
	}

	fmt.Println()
	fmt.Println("Results")
	fmt.Println("=======")
	fmt.Printf("Total requests:    %d\n", config.Requests)
	fmt.Printf("Successful:        %d (%.2f%%)\n", successCount, float64(successCount)/float64(config.Requests)*100)
	fmt.Printf("Failed:            %d (%.2f%%)\n", failCount, float64(failCount)/float64(config.Requests)*100)
	fmt.Println()

	if successCount > 0 {
		fmt.Println("Timing")
		fmt.Println("------")
		fmt.Printf("Min response time: %v\n", minTime)
		fmt.Printf("Max response time: %v\n", maxTime)
		fmt.Printf("Avg response time: %v\n", avgTime)
		fmt.Printf("Total time:        %v\n", totalTime)
		fmt.Printf("Requests/sec:      %.2f\n", float64(successCount)/totalTime.Seconds())
		fmt.Println()

		fmt.Println("Transfer")
		fmt.Println("--------")
		fmt.Printf("Total bytes:       %d\n", totalBytes)
		fmt.Printf("Avg bytes/request: %d\n", totalBytes/int64(successCount))
		fmt.Printf("Transfer rate:     %.2f KB/s\n", float64(totalBytes)/1024/totalTime.Seconds())
		fmt.Println()

		fmt.Println("Protocols")
		fmt.Println("---------")
		for proto, count := range protocols {
			fmt.Printf("%-10s: %d (%.2f%%)\n", proto, count, float64(count)/float64(successCount)*100)
		}
	}

	// Exit with error code if any requests failed
	if failCount > 0 {
		os.Exit(1)
	}
}
