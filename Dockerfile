# Build stage
FROM golang:alpine AS builder

# Set working directory inside the container
WORKDIR /app

# Copy go.mod to cache dependencies
COPY go.mod ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the Go application statically
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o redis-proxy main.go

# Runtime stage
FROM alpine:3.19

# Set working directory
WORKDIR /app

# Create a non-root user and group for security
RUN addgroup -S redisproxy && adduser -S redisproxy -G redisproxy

# Copy the compiled binary from the builder stage
COPY --from=builder /app/redis-proxy .

# Give the non-root user ownership of the application directory
RUN chown -R redisproxy:redisproxy /app

# Expose the default redis proxy listener port
EXPOSE 16379

# Run as the non-root user
USER redisproxy

# Define the entrypoint and default command
ENTRYPOINT ["./redis-proxy"]
CMD ["start", "-config", "/etc/redis-proxy/redis-proxy.conf"]
