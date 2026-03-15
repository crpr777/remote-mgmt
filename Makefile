.PHONY: all deps admin-ui client-darwin client-windows clean

VERSION ?= 1.0.0
AUTH_KEY ?= 

all: deps

deps:
	go mod tidy

# Run the admin UI server
admin-ui:
	cd web && go run server.go

# Build the admin UI server binary
build-admin:
	go build -o bin/admin-server ./web/

# Build client for macOS (Apple Silicon)
client-darwin-arm64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-darwin-arm64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		-o bin/remote-mgmt-client-darwin-arm64 \
		./cmd/client

# Build client for macOS (Intel)
client-darwin-amd64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-darwin-amd64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		-o bin/remote-mgmt-client-darwin-amd64 \
		./cmd/client

# Build client for Windows (64-bit)
client-windows-amd64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-windows-amd64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		-o bin/remote-mgmt-client-windows-amd64.exe \
		./cmd/client

# Build client for Windows (ARM64)
client-windows-arm64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-windows-arm64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)'" \
		-o bin/remote-mgmt-client-windows-arm64.exe \
		./cmd/client

# Build all client platforms
clients-all: client-darwin-arm64 client-darwin-amd64 client-windows-amd64 client-windows-arm64

clean:
	rm -rf bin/
	rm -rf /tmp/rmgmt-builds-*

# Development: run client locally (requires auth key)
run-client:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make run-client AUTH_KEY=tskey-auth-xxx)
endif
	TS_AUTHKEY=$(AUTH_KEY) go run ./cmd/client

# Show help
help:
	@echo "Remote Management Tool - Build Commands"
	@echo ""
	@echo "Setup:"
	@echo "  make deps                    - Download Go dependencies"
	@echo ""
	@echo "Admin UI:"
	@echo "  make admin-ui                - Run the admin UI server (localhost:8000)"
	@echo "  make build-admin             - Build admin server binary"
	@echo ""
	@echo "Client Builds (require AUTH_KEY):"
	@echo "  make client-darwin-arm64     - Build for macOS Apple Silicon"
	@echo "  make client-darwin-amd64     - Build for macOS Intel"
	@echo "  make client-windows-amd64    - Build for Windows 64-bit"
	@echo "  make client-windows-arm64    - Build for Windows ARM"
	@echo "  make clients-all             - Build for all platforms"
	@echo ""
	@echo "Example:"
	@echo "  make client-darwin-arm64 AUTH_KEY=tskey-auth-kXXXXXXXXXX"
	@echo ""
	@echo "Other:"
	@echo "  make clean                   - Remove built binaries"
	@echo "  make help                    - Show this help"
