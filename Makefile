SRC := $(shell find . -type f \( -name '*.go' -o -name 'AndroidManifest.xml' \); echo bound/bound.go)
BIN := $(shell echo "$$GOBIN")
BIND :=$(shell find bind -type f)

.PHONY: all
all: dist/client.apk \
	dist/native-server \
	dist/linux-client \
	dist/windows-client.exe

.PHONY: install
install: $(BIN)/homecam-server $(BIN)/homecam-client

.PHONY: install-mobile-client
install-mobile-client: vendor $(SRC)
	gomobile install -tags 'production mobile' -target=android/arm64 ./cmd/client

dist/client.apk: vendor $(SRC) | dist
	gomobile build -tags 'production mobile' -target=android/arm64 -o "$@" ./cmd/client

dist/osx-client: vendor $(SRC) | dist
	GOOS=darwin GOARCH=amd64 go build -tags 'production' -o "$@" ./cmd/client

dist/linux-client: vendor $(SRC) | dist
	GOOS=linux GOARCH=amd64 go build -i -tags 'production' -o "$@" ./cmd/client

dist/windows-client.exe: vendor $(SRC) | dist
	GOOS=windows GOARCH=amd64 go build -tags 'production' -o "$@" ./cmd/client

dist/native-server: vendor $(SRC) | dist
	go build -o "$@" ./cmd/server

$(BIN)/homecam-%: vendor $(SRC)
	go build -tags 'production' -i -o "$@" ./cmd/$*

vendor: $(SRC) go.mod go.sum
	GO111MODULE=on go mod vendor

go.sum:
	touch go.sum

dist:
	@mkdir dist 2>/dev/null

bound/bound.go: $(BIND) vendor
	go run ./cmd/bindata

.PHONY: reset
reset:
	rm -rf vendor
	rm -rf dist
	rm -f bound/bound.go

.PHONY: run-mobile-client
run-mobile-client: vendor
	go run -tags 'production mobile' ./cmd/client

.PHONY: run-desktop-client
run-desktop-client: vendor
	go run -tags 'production' ./cmd/client

.PHONY: run-server
run-server: vendor
	go run ./cmd/server

.PHONY: run-direct
run-direct: vendor
	go run ./cmd/direct

.PHONY: stress-client
stress-client: vendor
	for i in $$(seq 1 10); do \
		go run -tags 'production' ./cmd/client & \
	done; wait;
