# WebSurfer

WebSurfer is a high-performance API designed to help Large Language Models (LLMs) surf the web.

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

### Authentication

All API endpoints (except `/health`) require authentication via API key:

```bash
# Via Authorization header
-H "Authorization: Bearer YOUR_API_KEY"

# Or via X-API-Key header
-H "X-API-Key: YOUR_API_KEY"
```

### Fetch a URL

Endpoint: `POST /v1/fetch`

```bash
curl -X POST http://localhost:8080/v1/fetch \
  -H "Authorization: Bearer YOUR_API_KEY" \
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
