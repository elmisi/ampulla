FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o ampulla-mcp ./cmd/ampulla-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/ampulla-mcp /usr/local/bin/ampulla-mcp

EXPOSE 8765
ENTRYPOINT ["ampulla-mcp"]
