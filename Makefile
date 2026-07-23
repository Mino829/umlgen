.PHONY: build test vet clean cross-build

build:
	go build -o umlgen ./cmd/umlgen

test:
	go test ./...

vet:
	go vet ./...

cross-build:
	mkdir -p dist
	GOOS=darwin GOARCH=arm64 go build -o dist/umlgen-darwin-arm64 ./cmd/umlgen
	GOOS=darwin GOARCH=amd64 go build -o dist/umlgen-darwin-amd64 ./cmd/umlgen
	GOOS=linux GOARCH=amd64 go build -o dist/umlgen-linux-amd64 ./cmd/umlgen
	GOOS=windows GOARCH=amd64 go build -o dist/umlgen-windows-amd64.exe ./cmd/umlgen
