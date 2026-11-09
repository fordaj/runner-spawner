VERSION ?= dev
LDFLAGS ?= -trimpath -ldflags="-X main.toolVersion=$(VERSION)"
BIN := runner-spawner
DIST := dist

.PHONY: build test vet cross clean install fmt run

build:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN) .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w -l .

run: build
	./$(BIN) --help

install:
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN) .
	cp $(BIN) "$(GOPATH)/bin/runner-spawner" 2>/dev/null || cp $(BIN) /usr/local/bin/runner-spawner

cross:
	mkdir -p $(DIST)
	for target in \
		"darwin amd64" \
		"darwin arm64" \
		"linux amd64" \
		"linux arm64" \
		"windows amd64" \
		"windows arm64" \
	; do \
		set -- $$target; \
		arch="$$1-$$2"; \
		bin="runner-spawner"; \
		if [ "$$1" = "windows" ]; then bin="runner-spawner.exe"; fi; \
		mkdir -p "$(DIST)/$$arch"; \
		echo "building $$arch -> $(DIST)/$$arch/$$bin"; \
		CGO_ENABLED=0 GOOS=$$1 GOARCH=$$2 go build $(LDFLAGS) -o "$(DIST)/$$arch/$$bin" .; \
	done

clean:
	rm -rf $(DIST) $(BIN)
