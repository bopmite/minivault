FROM golang:1.23-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY src/ ./src/
RUN go build -o minivault ./src

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /build/minivault .
RUN mkdir -p /app/data
EXPOSE 3000
ENTRYPOINT ["/app/minivault"]
CMD ["-port", "3000", "-data", "/app/data"]
