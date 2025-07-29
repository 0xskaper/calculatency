FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install required packages for networking
RUN apk add --no-cache gcc musl-dev linux-headers

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o network-engine .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the binary
COPY --from=builder /app/network-engine .

# Expose port
EXPOSE 8080

# Add capabilities for raw sockets (alternative to running as root)
# Note: This requires Docker to be run with --cap-add=NET_RAW
CMD ["./network-engine"]
