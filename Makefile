APP_NAME ?= wherobots
GO ?= go
BIN_DIR ?= bin
BINARY ?= $(BIN_DIR)/$(APP_NAME)

.PHONY: all build test fmt tidy run clean

all: test build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BINARY) .

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

run:
	$(GO) run . $(ARGS)

clean:
	rm -rf $(BIN_DIR)
