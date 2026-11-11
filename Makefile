.PHONY: build test bench vet diagram clean

build:
	go build -o umlgen ./cmd/umlgen

test:
	go test ./...

bench:
	go test ./internal/benchmarks -run '^$$' -bench . -benchmem

vet:
	go vet ./...

diagram:
	mkdir -p docs/diagrams
	go run ./cmd/umlgen package . --language go --dependency-source imports \
		--include github.com/Mino829/umlgen --exclude testdata --exclude examples \
		--title 'umlgen package dependencies' --no-cache \
		--output docs/diagrams/umlgen-packages.puml

clean:
	rm -f umlgen
	rm -rf dist
