package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"golang.org/x/net/http2"
)

type TestResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	Error    string        `json:"error,omitempty"`
	Details  interface{}   `json:"details,omitempty"`
}

type BackendResponse struct {
	Server    string            `json:"server"`
	Timestamp time.Time         `json:"timestamp"`
	Path      string            `json:"path"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	Protocol  string            `json:"protocol"`
}

func main() {
	gatewayURL := flag.String("gateway", os.Getenv("GATEWAY_URL"), "Gateway URL")
	gatewayHTTPS := flag.String("gateway-https", os.Getenv("GATEWAY_HTTPS_URL"), "Gateway HTTPS URL")
	host := flag.String("host", "api.example.com", "Host header to use")
	verbose := flag.Bool("verbose", false, "Verbose output")
	flag.Parse()

	if *gatewayURL == "" {
		*gatewayURL = "http://localhost:8080"
	}
	if *gatewayHTTPS == "" {
		*gatewayHTTPS = "https://localhost:8443"
	}

	fmt.Println("=== HTTP Gateway Functional Tests ===")
	fmt.Printf("Gateway HTTP:  %s\n", *gatewayURL)
	fmt.Printf("Gateway HTTPS: %s\n", *gatewayHTTPS)
	fmt.Printf("Host header:   %s\n", *host)
	fmt.Println()

	results := []TestResult{}

	// Test 1: Basic HTTP Request
	results = append(results, testBasicHTTP(*gatewayURL, *host, *verbose))

	// Test 2: HTTP/2 Support
	results = append(results, testHTTP2(*gatewayHTTPS, *host, *verbose))

	// Test 3: Load Balancing
	results = append(results, testLoadBalancing(*gatewayURL, *host, *verbose))

	// Test 4: Different Paths
	results = append(results, testDifferentPaths(*gatewayURL, *host, *verbose))

	// Test 5: Multiple Hosts
	results = append(results, testMultipleHosts(*gatewayURL, *verbose))

	// Test 6: Health Check
	results = append(results, testHealthCheck(*gatewayURL, *verbose))

	// Print summary
	fmt.Println("\n=== Test Summary ===")
	passed := 0
	failed := 0
	for _, result := range results {
		status := "✓ PASS"
		if !result.Passed {
			status = "✗ FAIL"
			failed++
		} else {
			passed++
		}
		fmt.Printf("%s %s (%.2fms)\n", status, result.Name, float64(result.Duration.Microseconds())/1000)
		if result.Error != "" {
			fmt.Printf("       Error: %s\n", result.Error)
		}
	}
	fmt.Printf("\nTotal: %d, Passed: %d, Failed: %d\n", len(results), passed, failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func testBasicHTTP(gatewayURL, host string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "Basic HTTP Request"}

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequest("GET", gatewayURL+"/api/test", nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	req.Host = host

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected status code: %d", resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("failed to read response: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	var backendResp BackendResponse
	if err := json.Unmarshal(body, &backendResp); err != nil {
		result.Error = fmt.Sprintf("failed to parse response: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	if verbose {
		fmt.Printf("  Backend: %s, Path: %s\n", backendResp.Server, backendResp.Path)
	}

	result.Passed = true
	result.Duration = time.Since(start)
	result.Details = backendResp
	return result
}

func testHTTP2(gatewayURL, host string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "HTTP/2 Support"}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	// Enable HTTP/2 support
	http2.ConfigureTransport(transport)

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequest("GET", gatewayURL+"/api/test", nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	req.Host = host

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	if verbose {
		fmt.Printf("  Protocol: %s\n", resp.Proto)
	}

	// Check if HTTP/2 was used
	if resp.ProtoMajor != 2 {
		result.Error = fmt.Sprintf("expected HTTP/2, got %s", resp.Proto)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Duration = time.Since(start)
	result.Details = map[string]string{"protocol": resp.Proto}
	return result
}

func testLoadBalancing(gatewayURL, host string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "Load Balancing"}

	client := &http.Client{Timeout: 5 * time.Second}
	servers := make(map[string]int)
	requests := 10

	for i := 0; i < requests; i++ {
		req, err := http.NewRequest("GET", gatewayURL+"/api/test", nil)
		if err != nil {
			result.Error = fmt.Sprintf("failed to create request: %v", err)
			result.Duration = time.Since(start)
			return result
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			result.Error = fmt.Sprintf("request %d failed: %v", i, err)
			result.Duration = time.Since(start)
			return result
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var backendResp BackendResponse
		if err := json.Unmarshal(body, &backendResp); err == nil {
			servers[backendResp.Server]++
		}
	}

	if verbose {
		fmt.Printf("  Server distribution:\n")
		for server, count := range servers {
			fmt.Printf("    %s: %d requests\n", server, count)
		}
	}

	// Check that requests were distributed to multiple servers
	if len(servers) < 2 {
		result.Error = fmt.Sprintf("requests not distributed (only %d server(s) received traffic)", len(servers))
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Duration = time.Since(start)
	result.Details = servers
	return result
}

func testDifferentPaths(gatewayURL, host string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "Different Paths"}

	client := &http.Client{Timeout: 5 * time.Second}
	paths := []string{"/api/users", "/api/orders", "/api/products"}

	for _, path := range paths {
		req, err := http.NewRequest("GET", gatewayURL+path, nil)
		if err != nil {
			result.Error = fmt.Sprintf("failed to create request for %s: %v", path, err)
			result.Duration = time.Since(start)
			return result
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			result.Error = fmt.Sprintf("request to %s failed: %v", path, err)
			result.Duration = time.Since(start)
			return result
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var backendResp BackendResponse
		if err := json.Unmarshal(body, &backendResp); err != nil {
			result.Error = fmt.Sprintf("failed to parse response from %s: %v", path, err)
			result.Duration = time.Since(start)
			return result
		}

		if backendResp.Path != path {
			result.Error = fmt.Sprintf("path mismatch: expected %s, got %s", path, backendResp.Path)
			result.Duration = time.Since(start)
			return result
		}

		if verbose {
			fmt.Printf("  Path %s → Server %s\n", path, backendResp.Server)
		}
	}

	result.Passed = true
	result.Duration = time.Since(start)
	return result
}

func testMultipleHosts(gatewayURL string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "Multiple Hosts"}

	client := &http.Client{Timeout: 5 * time.Second}
	hosts := []string{"api.example.com", "www.example.com"}

	for _, host := range hosts {
		req, err := http.NewRequest("GET", gatewayURL+"/", nil)
		if err != nil {
			result.Error = fmt.Sprintf("failed to create request for %s: %v", host, err)
			result.Duration = time.Since(start)
			return result
		}
		req.Host = host

		resp, err := client.Do(req)
		if err != nil {
			result.Error = fmt.Sprintf("request to %s failed: %v", host, err)
			result.Duration = time.Since(start)
			return result
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			result.Error = fmt.Sprintf("unexpected status for %s: %d", host, resp.StatusCode)
			result.Duration = time.Since(start)
			return result
		}

		if verbose {
			fmt.Printf("  Host %s → Status %d\n", host, resp.StatusCode)
		}
	}

	result.Passed = true
	result.Duration = time.Since(start)
	return result
}

func testHealthCheck(gatewayURL string, verbose bool) TestResult {
	start := time.Now()
	result := TestResult{Name: "Health Check"}

	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(gatewayURL + "/health")
	if err != nil {
		result.Error = fmt.Sprintf("health check failed: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("unexpected status: %d", resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	if verbose {
		fmt.Printf("  Health check returned %d\n", resp.StatusCode)
	}

	result.Passed = true
	result.Duration = time.Since(start)
	return result
}
