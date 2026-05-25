# Determine root directory
ROOT_DIR=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Gather all .go files for use in dependencies below
GO_FILES=$(shell find $(ROOT_DIR) -name '*.go')

# Gather list of example binaries from cmd/
EXAMPLES=$(shell find $(ROOT_DIR)/cmd -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)

NILAWAY_FLAGS ?= -include-pkgs=github.com/blinklabs-io/gouroboros

.PHONY: mod-tidy build format golines test lint clean

mod-tidy:
	# Needed to fetch new dependencies and add them to go.mod
	go mod tidy

build: $(EXAMPLES)

clean:
	rm -f $(EXAMPLES)

format:
	go fmt ./...
	gofmt -s -w $(GO_FILES)

golines:
	golines -w --ignore-generated --chain-split-dots --max-len=80 --reformat-tags .

test: mod-tidy
	go test -v -race ./...

lint:
	golangci-lint run ./...

nilaway: mod-tidy ## Run nilaway nil safety analysis
	go run go.uber.org/nilaway/cmd/nilaway@latest $(NILAWAY_FLAGS) ./...

# Build example binaries
# Depends on GO_FILES to determine when rebuild is needed
$(EXAMPLES): $(GO_FILES)
	go build -o $(@) ./cmd/$(@)
