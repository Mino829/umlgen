.PHONY: build test bench vet clean

build:
	go build -o umlgen ./cmd/umlgen

test:
	go test ./...

bench:
	go test ./internal/benchmarks -run '^$$' -bench . -benchmem

vet:
	go vet ./...

clean:
	rm -f umlgen
	rm -rf dist
