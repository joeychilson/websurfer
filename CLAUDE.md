# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WebSurfer is a Go-based HTTP API for fetching and parsing web content in an LLM-friendly format. It provides intelligent web scraping with respect for robots.txt, rate limiting, caching (stale-while-revalidate), and content transformation optimized for large language model consumption.

## Commands

### Building and Running

```bash
# Build the server
go build -o server ./cmd/server

# Run the server (uses config.yaml by default)
go run ./cmd/server/main.go

# Run with custom configuration
go run ./cmd/server/main.go -config ./config.yaml -addr :8080

# Run with specific cache backend
go run ./cmd/server/main.go -cache redis -redis-url redis://localhost:6379/0

# Run with debug logging
go run ./cmd/server/main.go -log-level debug

# Run with Docker
docker build -t websurfer .
docker run -p 8080:8080 websurfer
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for a specific package
go test ./api
go test ./client
go test ./parser/html

# Run a specific test
go test ./api -run TestHandleFetch
```

### Development

```bash
# Install dependencies
go mod download

# Format code
go fmt ./...

# Run go vet
go vet ./...

# Update dependencies
go mod tidy
```

## Architecture

### Request Flow

1. **API Layer** (`api/`): Chi router with middleware (logging, rate limiting, recovery)
   - `/fetch` (POST): Fetch and parse a single URL with optional truncation/range selection
   - `/map` (POST): Discover and map URLs from a website (sitemap or link crawling)
   - `/health` (GET): Health check endpoint

2. **Client Layer** (`client/`): Central orchestrator that coordinates all components
   - Checks robots.txt permissions via `robots.Checker`
   - Applies rate limiting per domain via `ratelimit.Limiter`
   - Implements stale-while-revalidate caching via `cache.Cache`
   - Delegates HTTP fetching to `fetcher.Fetcher`
   - Wraps fetcher with `retry.Retry` for transient failure handling
   - Parses content using registered parsers from `parser.Registry`

3. **Core Components**:
   - **Fetcher** (`fetcher/`): Low-level HTTP client with URL rewriting, alternative format checking (e.g., /llms.txt, .md), and redirect handling
   - **Parser** (`parser/`): Registry-based content transformation system. HTML parser uses bluemonday for sanitization and custom rules for site-specific cleanup (e.g., SEC.gov EDGAR tables)
   - **Robots** (`robots/`): Parses robots.txt, checks crawl permissions, extracts sitemap URLs, respects crawl-delay directives
   - **Rate Limiter** (`ratelimit/`): Per-domain rate limiting with support for Retry-After headers and crawl-delay integration
   - **Cache** (`cache/`): Dual implementation (in-memory, Redis) with TTL and stale-time support for stale-while-revalidate pattern
   - **Retry** (`retry/`): Exponential backoff retry logic for transient HTTP errors

4. **Content Processing** (`content/`):
   - Token estimation and truncation for LLM context limits
   - Range extraction (lines/chars) for pagination

### Configuration System

The `config/` package implements a hierarchical configuration system:

- **Default config**: Applied to all requests (defined in `config.yaml` or programmatically)
- **Site-specific overrides**: URL pattern matching (e.g., `*.sec.gov`) with partial overrides for cache, fetch, rate_limit, and retry settings
- **Merge strategy**: Site configs override only specified fields, inheriting defaults for unset values
- **`ResolvedConfig`**: Final merged config used by the client for each URL

Key configuration capabilities:
- User-Agent customization per site
- Timeout, redirect behavior, robots.txt respect
- Rate limiting (requests/second, burst, crawl-delay integration)
- Cache TTL and stale-time
- Retry parameters (max attempts, delays)
- URL rewrites (string replacement or regex)
- Format checking (try /llms.txt or .md before fetching original URL)

### Parser System

The parser system (`parser/`) is extensible and content-type aware:

- **Parser Registry**: Maps content-types (e.g., "text/html", "application/xhtml+xml") to parser implementations
- **HTML Parser** (`parser/html/`):
  - Uses bluemonday for HTML sanitization
  - Applies custom rules for site-specific transformations
  - Example: `parser/rules/sec.go` handles SEC.gov EDGAR document formatting, preserving table structure while cleaning noise
- **Context-aware**: URLs can be passed via context to parsers for intelligent processing

Current parsers registered by default:
- HTML parser with SEC.gov-specific rules

### Caching Strategy

Implements **stale-while-revalidate** pattern:

1. If cached content is **fresh** (within TTL): return immediately
2. If cached content is **stale** (TTL expired but within stale_time): return stale content, trigger background refresh
3. If cache miss or content too old: fetch synchronously

Cache implementations:
- **Memory cache** (`cache/memory.go`): LRU-based in-memory cache with TTL/stale-time support
- **Redis cache** (`cache/redis.go`): Distributed cache with TTL/stale-time metadata

### Security

- **SSRF Protection** (`api/handlers.go:617`): `validateNotInternalURL` prevents requests to private/loopback IPs
- **Robots.txt respect**: Enforced by default (configurable per site)
- **Rate limiting**: Both client-side (per-domain) and API-level (per-requester IP)

## Important Patterns

### Site-Specific Customization

To add support for a new site with special requirements:

1. Add a site pattern in `config.yaml` under `sites:`
2. If special parsing is needed, create a new rule in `parser/rules/` implementing the parsing logic
3. Register the rule in `client/client.go` when building the HTML parser

Example: SEC.gov configuration in `config.yaml:28-42` with custom parsing in `parser/rules/sec.go`

### Testing Strategy

- Each package has comprehensive `_test.go` files
- Tests use table-driven approach with subtests
- HTTP tests use `httptest` for mocking
- Cache tests use miniredis for Redis mocking
- Parser tests include real-world HTML samples

### API Response Format

All API responses include:
- **Metadata**: URL, status code, content type, title, description, cache state, estimated tokens
- **Content**: The processed/parsed content
- **NextRange** (optional): For paginated responses when content is truncated

Map endpoint returns:
- **Source**: "sitemap" or "html_links" (indicates discovery method)
- **Pages**: Array of URLs with metadata (title, description, noindex flag)

## Environment Variables

The server respects these environment variables (can also be set via flags):
- `ADDR`: HTTP server address (default: `:8080`)
- `CONFIG_FILE`: Path to config file (default: `./config.yaml`)
- `CACHE_TYPE`: Cache backend: `none`, `memory`, or `redis` (default: `memory`)
- `REDIS_URL`: Redis connection URL (default: `redis://localhost:6379/0`)
- `LOG_LEVEL`: Log level: `debug`, `info`, `warn`, `error` (default: `info`)
