# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
# If you have a go.sum, uncomment the next line
# COPY go.sum ./ 
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Stage 2: Final lightweight image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the binary from the builder
COPY --from=builder /app/main .

# Copy the frontend and data folders
COPY --from=builder /app/index.html .
COPY --from=builder /app/tests ./tests

# Expose the port your app runs on (change 8080 if needed)
EXPOSE 8080

CMD ["./main"]