BIN ?= bin

.PHONY: test cli server clean

test:
	go test ./...

cli:
	go build -o $(BIN)/qris ./cmd/qris

server:
	go build -o $(BIN)/qris-server ./cmd/qris-server

clean:
	rm -rf $(BIN)
