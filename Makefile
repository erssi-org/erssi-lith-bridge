.PHONY: build run clean test

# Build the bridge
build:
	go build -o erssi-lith-bridge ./cmd/bridge

# Run the bridge (development)
run:
	go run ./cmd/bridge -erssi ws://localhost:9001 -listen :9000 -v

# Clean build artifacts
clean:
	rm -f erssi-lith-bridge

# Run tests
test:
	go test -v ./...

# Format code
fmt:
	go fmt ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o erssi-lith-bridge-linux-amd64 ./cmd/bridge
	GOOS=darwin GOARCH=amd64 go build -o erssi-lith-bridge-darwin-amd64 ./cmd/bridge
	GOOS=darwin GOARCH=arm64 go build -o erssi-lith-bridge-darwin-arm64 ./cmd/bridge
	GOOS=windows GOARCH=amd64 go build -o erssi-lith-bridge-windows-amd64.exe ./cmd/bridge
