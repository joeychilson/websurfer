# WebSurfer

WebSurfer is a high-performance API designed to help Large Language Models (LLMs) surf the web. It fetches, parses, and processes web content (HTML, PDF, Markdown) into LLM-friendly formats with built-in token estimation, pagination, and caching.

## Features

- **Smart Fetching**: Handles dynamic content, redirects, and various content types.
- **Content Parsing**: Converts HTML and PDF to clean Markdown text.
- **Token Awareness**: Estimates tokens and supports pagination (offsets/limits) to fit context windows.
- **Caching**: Redis-backed caching with configurable TTLs to speed up repeated requests.
- **Politeness**: Respects `robots.txt` (configurable), rate limits requests, and handles retries gracefully.
- **Configurable**: Site-specific rules for timeouts, caching strategies, and user agents.

## Prerequisites

- **Go** (1.22 or later)
- **Redis** (required for caching)

## Getting Started

1. **Clone the repository**

   ```bash
   git clone https://github.com/joeychilson/websurfer.git
   cd websurfer
   ```

2. **Start Redis**
   Ensure you have a Redis server running.

   ```bash
   redis-server
   ```

3. **Run the Server**
   You can run the server directly using Go:

   ```bash
   export REDIS_URL="redis://localhost:6379/0"
   go run cmd/server/main.go
   ```

   Or build it first:

   ```bash
   go build -o websurfer cmd/server/main.go
   ./websurfer
   ```

   By default, the server listens on port `8080`.

## Configuration

WebSurfer uses a `config.yaml` file for detailed behavior and environment variables for connection settings.

### Environment Variables

- `ADDR`: Server address (default `:8080`)
- `REDIS_URL`: Redis connection URL (required, e.g., `redis://localhost:6379/0`)
- `CONFIG_FILE`: Path to config file (default `./config.yaml`)
- `LOG_LEVEL`: Logging level (`debug`, `info`, `warn`, `error`)

### Config File

See `config.yaml` to tune:

- Global cache TTLs
- User Agents
- Rate limits (requests per second, burst)
- Site-specific patterns (e.g., distinct rules for `*.sec.gov` or `docs.*`)

## Usage

### Fetch a URL

Endpoint: `POST /v1/fetch`

```bash
curl -X POST http://localhost:8080/v1/fetch \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "max_tokens": 1000
  }'
```

**Response:**

```json
{
  "metadata": {
    "url": "https://example.com",
    "status_code": 200,
    "content_type": "text/html; charset=UTF-8",
    "title": "Example Domain",
    "estimated_tokens": 150,
    "cache_state": "miss"
  },
  "content": "Example Domain\n\nThis domain is for use in illustrative examples...",
  "pagination": {
    "offset": 0,
    "limit": 1000,
    "total_tokens": 150,
    "has_more": false
  }
}
```

### Health Check

Endpoint: `GET /health`

```bash
curl http://localhost:8080/health
```

## License

See [LICENSE](LICENSE) file for details.
