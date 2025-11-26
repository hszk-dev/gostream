FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/worker ./cmd/worker

FROM alpine:3.19

# Install FFmpeg for video transcoding
RUN apk add --no-cache ca-certificates tzdata ffmpeg

WORKDIR /app

COPY --from=builder /app/worker .

# Create temp directory for transcoding
RUN mkdir -p /tmp/gostream

CMD ["./worker"]
