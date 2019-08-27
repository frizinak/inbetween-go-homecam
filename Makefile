SRC := $(shell find . -type f \( -name '*.go' -o -name 'AndroidManifest.xml' -o -name 'credentials' \))

client.apk: vendor $(SRC) credentials
	@gomobile build -ldflags "$(shell cat credentials)"  -o $@ ./cmd/client

.PHONY: install-client
install-client: vendor credentials
	@gomobile install -ldflags '$(shell cat credentials)' ./cmd/client

credentials: | credentials.example
	cp credentials.example $@

vendor: $(SRC) go.mod go.sum
	GO111MODULE=on go mod vendor

go.sum:
	touch go.sum

.PHONY: run-client
run-client: vendor
	go run ./cmd/client

.PHONY: run-server
run-server: vendor
	go run ./cmd/server

.PHONY: run-direct
run-direct: vendor
	go run ./cmd/direct

.PHONY: stress-client
stress-client: vendor
	for i in $$(seq 1 10); do \
		go run ./cmd/client & \
	done; wait;

.PHONY: run-built-client
run-built-client: vendor credentials
	@go build -ldflags '$(shell cat credentials)' -o $@ ./cmd/client
	./$@

