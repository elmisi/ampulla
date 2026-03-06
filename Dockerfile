FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o ampulla ./cmd/ampulla

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/ampulla /usr/local/bin/ampulla

EXPOSE 8090
CMD ["ampulla"]
