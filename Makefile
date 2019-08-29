SRC := $(shell find . -type f \( -name '*.go' -o -name 'AndroidManifest.xml' \))

dist/client.apk: vendor $(SRC) | dist
	gomobile build -target=android/arm64 -o $@ ./cmd/client

.PHONY: all
all: dist/client.apk dist/native-server dist/native-client

.PHONY: install-mobile-client
install-mobile-client: vendor $(SRC)
	gomobile install -tags 'production' -target=android/arm64 ./cmd/client

dist/native-client: vendor $(SRC) | dist
	go build -tags 'production' -o $@ ./cmd/client

dist/native-server: vendor $(SRC) | dist
	go build -o $@ ./cmd/server

vendor: $(SRC) go.mod go.sum
	GO111MODULE=on go mod vendor

go.sum:
	touch go.sum

dist:
	@mkdir dist 2>/dev/null

.PHONY: run-client
run-client: vendor
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
