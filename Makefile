GO   ?= go
BIN  ?= assetlas

.PHONY: build scrape enum readme fmt vet test tools clean

build:
	$(GO) build -o $(BIN) .

scrape: build
	./$(BIN) scrape

enum: build
	./$(BIN) enum

readme: build
	./$(BIN) readme

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test -race ./...

tools:
	$(GO) install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
	$(GO) install github.com/projectdiscovery/httpx/cmd/httpx@latest
	$(GO) install github.com/projectdiscovery/naabu/v2/cmd/naabu@latest
	$(GO) install github.com/projectdiscovery/dnsx/cmd/dnsx@latest
	$(GO) install github.com/projectdiscovery/tlsx/cmd/tlsx@latest

clean:
	rm -f $(BIN)
