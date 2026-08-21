.PHONY: all build build-linux-amd64 build-linux-arm64 test lint run docker checksums clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "unknown")

LDFLAGS := -s -w \
	-X 'github.com/liteploy/liteploy/internal/system.Version=$(VERSION)' \
	-X 'github.com/liteploy/liteploy/internal/system.CommitSHA=$(COMMIT)' \
	-X 'github.com/liteploy/liteploy/internal/system.BuildDate=$(BUILD_DATE)'

all: build

build:
	go build -ldflags="$(LDFLAGS)" -o bin/liteploy ./cmd/liteploy

build-linux-amd64:
	mkdir -p bin/
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/liteploy-linux-amd64 ./cmd/liteploy

build-linux-arm64:
	mkdir -p bin/
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o bin/liteploy-linux-arm64 ./cmd/liteploy

checksums: build-linux-amd64 build-linux-arm64
	cd bin && sha256sum liteploy-linux-amd64 liteploy-linux-arm64 > checksums.txt

test:
	go test -v ./...

lint:
	go vet ./...

run: build
	LITEPLOY_DEV_MODE=true ./bin/liteploy

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t liteploy:latest .

clean:
	rm -rf bin/
