FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o server \
    ./cmd/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata poppler-utils \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ttf-freefont

ENV CHROME_BIN=/usr/bin/chromium-browser \
    CHROME_PATH=/usr/lib/chromium/

RUN addgroup -g 1000 websurfer && \
    adduser -D -u 1000 -G websurfer websurfer

WORKDIR /app

COPY --from=builder /build/server .

COPY config.yaml .

USER websurfer

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/server", "-config", "/app/config.yaml"]
