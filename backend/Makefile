# Load variables from .env if it exists
ifneq (,$(wildcard .env))
	include .env
	export $(shell sed 's/=.*//' .env)
endif

BINARY_NAME=flox-backend
GO_MODULE=github.com/cheathuber/flox-backend
GOOS=linux
GOARCH=amd64
CGO_ENABLED=0

# Get the latest git tag or commit hash
VERSION := $(shell git describe --tags --always --dirty)

BUILD_DIR=build
DEB_DIR=$(BUILD_DIR)/$(BINARY_NAME)_$(VERSION)_amd64
TEMPLATE_DIR=templates
SCRIPT_DIR=scripts

# Default target
all: clean package upload

# Build the binary with static linking
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -ldflags="-X main.Version=$(VERSION)" -o $(BINARY_NAME)
	@echo "Build completed: $(BINARY_NAME)"

# Clean the binary
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	@echo "Clean completed."

package-prepare: build
	mkdir -p $(DEB_DIR)/DEBIAN
	install -m 755 -D $(BINARY_NAME) $(DEB_DIR)/usr/local/bin/$(BINARY_NAME)
	install -m 644 -D debian/$(BINARY_NAME).service $(DEB_DIR)/etc/systemd/system/$(BINARY_NAME).service
	install -m 644 -D debian/$(BINARY_NAME).1 $(DEB_DIR)/usr/share/man/man1/$(BINARY_NAME).1
	install -m 644 -D config.yaml.example $(DEB_DIR)/etc/flox/backend.example.yaml

	
	# Install scripts
	mkdir -p $(DEB_DIR)/usr/lib/flox/scripts
	install -m 755 $(SCRIPT_DIR)/create_site.sh $(DEB_DIR)/usr/lib/flox/scripts/

	# Install maintainer scripts
	sed "s/@VERSION@/$(VERSION)/g" < debian/control.template > $(DEB_DIR)/DEBIAN/control
	install -m 755 -D debian/postinst $(DEB_DIR)/DEBIAN/postinst
	install -m 755 -D debian/prerm $(DEB_DIR)/DEBIAN/prerm

	# Set permissions
	find $(DEB_DIR) -type d -exec chmod 0755 {} \;
	find $(DEB_DIR)/usr/lib/flox/scripts -type f -exec chmod 0744 {} \;
	chmod 0640 $(DEB_DIR)/etc/flox/backend.example.yaml


package: package-prepare
	@echo "Packaging $(BINARY_NAME)..."
	dpkg-deb --build --root-owner-group $(DEB_DIR)
	@echo "Created $(BINARY_NAME)_$(VERSION)_amd64.deb"

upload:
	@echo "Checking for DEB_REGISTRY..."
	@if [ -n "$(DEB_REGISTRY)" ]; then \
		echo "Uploading to registry $(DEB_REGISTRY)"; \
		./upload-deb.sh $(BUILD_DIR)/$(BINARY_NAME)_$(VERSION)_amd64.deb; \
	else \
		echo "DEB_REGISTRY not set, skipping upload"; \
	fi
	
# Run the binary
run: build
	@echo "Running the binary..."
	./$(BINARY_NAME)

# Install locally (for testing)
install-local: package
	sudo dpkg -i $(BUILD_DIR)/$(BINARY_NAME)_$(VERSION)_amd64.deb

# Uninstall package
uninstall:
	sudo dpkg --purge flox-backend

.PHONY: all build clean package-prepare package upload run install-local uninstall
