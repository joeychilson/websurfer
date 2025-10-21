package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/joeychilson/websurfer/api"
)

const (
	defaultServerURL = "http://localhost:8080"
)

type BenchmarkResult struct {
	Metadata      api.Metadata  `json:"metadata"`
	TimeTaken     time.Duration `json:"time_taken_ms"`
	ContentLength int           `json:"content_length"`
	RequestTime   string        `json:"request_time"`
}

func main() {
	serverURL := flag.String("server", defaultServerURL, "Server URL")
	url := flag.String("url", "", "URL to fetch (required)")
	jsonOutput := flag.Bool("json", false, "Output as JSON")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "WebSurfer Benchmark Tool - Fetch URL and return metadata with timing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com -server http://localhost:3000\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -url https://example.com -json\n", os.Args[0])
	}

	flag.Parse()

	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: -url flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	result, err := benchmarkFetch(*serverURL, *url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(result)
	} else {
		outputHuman(result)
	}
}

func benchmarkFetch(serverURL, targetURL string) (*BenchmarkResult, error) {
	reqBody := api.FetchRequest{
		URL: targetURL,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := serverURL + "/fetch"

	start := time.Now()
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	timeTaken := time.Since(start)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var fetchResp api.FetchResponse
	if err := json.Unmarshal(body, &fetchResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &BenchmarkResult{
		Metadata:      fetchResp.Metadata,
		TimeTaken:     timeTaken,
		ContentLength: len(fetchResp.Content),
		RequestTime:   time.Now().Format(time.RFC3339),
	}, nil
}

func outputJSON(result *BenchmarkResult) {
	output := map[string]interface{}{
		"metadata":       result.Metadata,
		"time_taken_ms":  result.TimeTaken.Milliseconds(),
		"content_length": result.ContentLength,
		"request_time":   result.RequestTime,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

func outputHuman(result *BenchmarkResult) {
	fmt.Println("=== WebSurfer Benchmark Results ===")
	fmt.Println()
	fmt.Printf("URL:              %s\n", result.Metadata.URL)
	fmt.Printf("Status Code:      %d\n", result.Metadata.StatusCode)
	fmt.Printf("Content Type:     %s\n", result.Metadata.ContentType)

	if result.Metadata.Title != "" {
		fmt.Printf("Title:            %s\n", result.Metadata.Title)
	}

	if result.Metadata.Description != "" {
		descLimit := 100
		desc := result.Metadata.Description
		if len(desc) > descLimit {
			desc = desc[:descLimit] + "..."
		}
		fmt.Printf("Description:      %s\n", desc)
	}

	if result.Metadata.Language != "" {
		fmt.Printf("Language:         %s\n", result.Metadata.Language)
	}

	if result.Metadata.LastModified != "" {
		fmt.Printf("Last Modified:    %s\n", result.Metadata.LastModified)
	}

	fmt.Printf("Estimated Tokens: %d\n", result.Metadata.EstimatedTokens)
	fmt.Printf("Content Length:   %d bytes\n", result.ContentLength)

	if result.Metadata.CacheState != "" {
		fmt.Printf("Cache State:      %s\n", result.Metadata.CacheState)
	}

	if result.Metadata.CachedAt != "" {
		fmt.Printf("Cached At:        %s\n", result.Metadata.CachedAt)
	}

	fmt.Println()
	fmt.Printf("⏱️  Time Taken:      %v\n", result.TimeTaken)
	fmt.Printf("📅 Request Time:    %s\n", result.RequestTime)
}
