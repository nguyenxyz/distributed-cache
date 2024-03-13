FROM golang:latest

# Set the working directory inside the container
WORKDIR /app

# Copy the Go module files first
COPY go.mod go.sum ./

# Download Go module dependencies
RUN go mod download

# Copy the entire source code
COPY . .

# Build the Go application
RUN go build -o api ./cmd/apiserver

# Expose the port your app runs on (optional)
EXPOSE 9000

# Set the entry point to run the executable
CMD ["./api"]