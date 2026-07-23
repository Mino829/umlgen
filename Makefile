.PHONY: build test vet clean

build:
	go build -o umlgen ./cmd/umlgen

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -f umlgen
	rm -rf dist
