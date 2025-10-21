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
	Metadata      api.Metadata     `json:"metadata,omitempty"`
	TimeTaken     time.Duration    `json:"time_taken_ms"`
	ContentLength int              `json:"content_length,omitempty"`
	RequestTime   string           `json:"request_time"`
	MapResponse   *api.MapResponse `json:"map_response,omitempty"`
	URLCount      int              `json:"url_count,omitempty"`
}

func main() {
	serverURL := flag.String("server", defaultServerURL, "Server URL")
	url := flag.String("url", "", "URL to fetch/map (required)")
	mode := flag.String("mode", "fetch", "Mode: 'fetch' or 'map'")
	jsonOutput := flag.Bool("json", false, "Output as JSON")
	maxURLs := flag.Int("max-urls", 100, "Maximum URLs to return (map mode only)")
	pathPrefix := flag.String("path-prefix", "", "Filter URLs by path prefix (map mode only)")
	sameDomain := flag.Bool("same-domain", false, "Only return URLs from same domain (map mode only)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "WebSurfer Benchmark Tool - Fetch or Map URLs with timing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  Fetch mode:\n")
		fmt.Fprintf(os.Stderr, "    %s -url https://example.com\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -url https://example.com -json\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n  Map mode:\n")
		fmt.Fprintf(os.Stderr, "    %s -mode map -url https://example.com\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -mode map -url https://example.com -max-urls 50\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -mode map -url https://example.com -path-prefix /blog\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -mode map -url https://example.com -path-prefix /docs -max-urls 200\n", os.Args[0])
	}

	flag.Parse()

	if *url == "" {
		fmt.Fprintf(os.Stderr, "Error: -url flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	var result *BenchmarkResult
	var err error

	switch *mode {
	case "fetch":
		result, err = benchmarkFetch(*serverURL, *url)
	case "map":
		result, err = benchmarkMap(*serverURL, *url, *maxURLs, *pathPrefix, *sameDomain)
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid mode '%s'. Must be 'fetch' or 'map'\n\n", *mode)
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		outputJSON(result)
	} else {
		outputHuman(result, *mode)
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

func benchmarkMap(serverURL, targetURL string, maxURLs int, pathPrefix string, sameDomain bool) (*BenchmarkResult, error) {
	reqBody := api.MapRequest{
		URL:        targetURL,
		MaxURLs:    maxURLs,
		PathPrefix: pathPrefix,
		SameDomain: sameDomain,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := serverURL + "/map"

	start := time.Now()
	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to map: %w", err)
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

	var mapResp api.MapResponse
	if err := json.Unmarshal(body, &mapResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &BenchmarkResult{
		TimeTaken:   timeTaken,
		RequestTime: time.Now().Format(time.RFC3339),
		MapResponse: &mapResp,
		URLCount:    mapResp.Count,
	}, nil
}

func outputJSON(result *BenchmarkResult) {
	output := map[string]interface{}{
		"time_taken_ms": result.TimeTaken.Milliseconds(),
		"request_time":  result.RequestTime,
	}

	if result.MapResponse != nil {
		output["map_response"] = result.MapResponse
		output["url_count"] = result.URLCount
	} else {
		output["metadata"] = result.Metadata
		output["content_length"] = result.ContentLength
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

func outputHuman(result *BenchmarkResult, mode string) {
	if mode == "map" {
		fmt.Println("=== WebSurfer Map Benchmark Results ===")
		fmt.Println()
		fmt.Printf("Base URL:         %s\n", result.MapResponse.BaseURL)
		fmt.Printf("Source:           %s\n", result.MapResponse.Source)
		fmt.Printf("URLs Found:       %d\n", result.MapResponse.Count)
		fmt.Printf("Truncated:        %v\n", result.MapResponse.Truncated)
		fmt.Println()

		if result.MapResponse.Count > 0 {
			displayLimit := 10
			if result.MapResponse.Count < displayLimit {
				displayLimit = result.MapResponse.Count
			}

			fmt.Printf("First %d URLs:\n", displayLimit)
			for i, url := range result.MapResponse.URLs[:displayLimit] {
				fmt.Printf("  %2d. %s\n", i+1, url)
			}

			if result.MapResponse.Count > displayLimit {
				fmt.Printf("  ... and %d more\n", result.MapResponse.Count-displayLimit)
			}
		}

		fmt.Println()
		fmt.Printf("⏱️  Time Taken:      %v\n", result.TimeTaken)
		fmt.Printf("📅 Request Time:    %s\n", result.RequestTime)
	} else {
		fmt.Println("=== WebSurfer Fetch Benchmark Results ===")
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
}
