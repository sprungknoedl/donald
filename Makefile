.PHONY: build check fmt vet test all clean

build:
	go build -o donald

# Canonical verification: run this (and CI runs it) before trusting a change.
check: build vet test
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi
	@echo "✓ check passed"

fmt:
	gofmt -w $$(find . -name '*.go')

vet:
	go vet ./...

test:
	go test ./...

all:
	GOOS=linux GOARCH=amd64 go build -o donald-linux-amd64
	GOOS=linux GOARCH=arm go build -o donald-linux-arm
	GOOS=linux GOARCH=arm64 go build -o donald-linux-arm64
	GOOS=darwin GOARCH=amd64 go build -o donald-mac-amd64
	GOOS=darwin GOARCH=arm64 go build -o donald-mac-arm64
	GOOS=windows GOARCH=amd64 go build -o donald-windows-amd64.exe
	GOOS=windows GOARCH=arm64 go build -o donald-windows-arm64.exe

clean:
	rm -f donald-linux-amd64 donald-linux-arm donald-linux-arm64 \
		donald-mac-amd64 donald-mac-arm64 \
		donald-windows-amd64.exe donald-windows-arm.exe donald-windows-arm64.exe