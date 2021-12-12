BINARY=go-ouroboros-network

# Determine root directory
ROOT_DIR=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Gather all .go files for use in dependencies below
GO_FILES=$(shell find $(ROOT_DIR) -name '*.go')

# Build our program binary
# Depends on GO_FILES to determine when rebuild is needed
$(BINARY): $(GO_FILES)
	# Needed to fetch new dependencies and add them to go.mod
	go mod tidy
	go build -o $(BINARY) ./cmd/$(BINARY)

.PHONY: build image

# Alias for building program binary
build: $(BINARY)

clean:
	rm -f $(BINARY)
