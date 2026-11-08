VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test build-linux-amd64 build-windows-amd64 release

test:
	gofmt -w .
	go vet ./...
	go test -race ./...

build-linux-amd64:
	mkdir -p dist/linux-amd64
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-amd64/ichtuansu .

build-windows-amd64:
	mkdir -p dist/windows-amd64
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$(DLFLAGS)" -o dist/windows-amd64/_ichuansu.exe .
	cp ichuansu.cmd dist/windows-amd64/ichuansu.cmd

release: build-linux-amd64 build-windows-amd64