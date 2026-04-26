# 1. Start with a lightweight version of Linux that has Go pre-installed
FROM golang:alpine

# 2. IMPORTANT: SQLite requires a C compiler to work in Go. 
# We must install 'gcc' and 'musl-dev' into the Linux container.
RUN apk add --no-cache gcc musl-dev

# 3. Create a working directory inside the container
WORKDIR /app

# 4. Copy your Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# 5. Copy the rest of your code (HTML, JS, CSS, main.go, roster.go)
COPY . .

# 6. Build the Go application! 
# CGO_ENABLED=1 is strictly required for the SQLite driver.
RUN CGO_ENABLED=1 GOOS=linux go build -o fred-server .

# 7. Expose the port your app runs on
EXPOSE 8080

# 8. Start the server when the container boots
CMD ["./fred-server"]
