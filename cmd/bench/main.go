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

	"github.com/joeychilson/websurfer/search"
	api "github.com/joeychilson/websurfer/server"
)

const (
	defaultServerURL = "http://localhost:8080"
)

type BenchmarkResult struct {
	Metadata      api.Metadata   `json:"metadata,omitempty"`
	TimeTaken     time.Duration  `json:"time_taken_ms"`
	ContentLength int            `json:"content_length,omitempty"`
	RequestTime   string         `json:"request_time"`
	Content       string         `json:"content,omitempty"`
	SearchResults *search.Result `json:"search_results,omitempty"`
}

func main() {
	serverURL := flag.String("server", defaultServerURL, "Server URL")
	url := flag.String("url", "", "URL to fetch (required)")
	jsonOutput := flag.Bool("json", false, "Output as JSON")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "WebSurfer Benchmark Tool - Fetch URLs with timing\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "    %s -url https://example.com\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "    %s -url https://example.com -json\n", os.Args[0])
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
		Content:       fetchResp.Content,
		SearchResults: fetchResp.SearchResults,
	}, nil
}

func outputJSON(result *BenchmarkResult) {
	output := map[string]interface{}{
		"time_taken_ms":  result.TimeTaken.Milliseconds(),
		"request_time":   result.RequestTime,
		"metadata":       result.Metadata,
		"content_length": result.ContentLength,
	}

	if result.SearchResults != nil {
		output["search_results"] = result.SearchResults
	} else {
		output["content"] = result.Content
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(jsonData))
}

func outputHuman(result *BenchmarkResult) {
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

	fmt.Println()

	if result.SearchResults != nil {
		fmt.Println("=== Search Results ===")
		fmt.Println()
		fmt.Printf("Query:            %s\n", result.SearchResults.Query)
		fmt.Printf("Total Matches:    %d\n", result.SearchResults.TotalMatches)
		fmt.Printf("Returned Matches: %d\n", result.SearchResults.ReturnedMatches)
		fmt.Println()

		for i, match := range result.SearchResults.Results {
			fmt.Printf("--- Result #%d (Rank: %d, Score: %.2f) ---\n", i+1, match.Rank, match.Score)

			if match.Location.SectionPath != "" {
				fmt.Printf("Section:          %s\n", match.Location.SectionPath)
			}

			fmt.Printf("Location:         chars %d-%d, lines %d-%d\n",
				match.Location.CharStart, match.Location.CharEnd,
				match.Location.LineStart, match.Location.LineEnd)
			fmt.Printf("Estimated Tokens: %d\n", match.EstimatedTokens)
			fmt.Println()

			fmt.Printf("Snippet:\n%s\n", truncateForDisplay(match.Snippet, 500))
			fmt.Println()
		}
	} else {
		fmt.Println("=== Content ===")
		fmt.Println()
		fmt.Printf("%s\n", result.Content)
	}
}

// truncateForDisplay truncates text for human-readable display
func truncateForDisplay(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
