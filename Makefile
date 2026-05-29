# Determine root directory
ROOT_DIR=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))

# Gather all .go files for use in dependencies below
GO_FILES=$(shell find $(ROOT_DIR) -name '*.go')
GO_MOD_FILES=$(ROOT_DIR)/go.mod $(ROOT_DIR)/go.sum

# Gather list of example binaries from examples/
EXAMPLE_DIR=$(ROOT_DIR)/examples
EXAMPLES=$(shell find $(EXAMPLE_DIR) -mindepth 1 -maxdepth 1 -type d -exec basename {} \;)

NILAWAY_FLAGS ?= -include-pkgs=github.com/blinklabs-io/gouroboros

.PHONY: mod-tidy build format golines test lint clean

mod-tidy:
	# Needed to fetch new dependencies and add them to go.mod
	go mod tidy
	@set -e; for example in $(EXAMPLES); do \
		echo "tidy examples/$$example"; \
		go -C $(EXAMPLE_DIR)/$$example mod tidy; \
	done

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
	@set -e; for example in $(EXAMPLES); do \
		echo "test examples/$$example"; \
		go -C $(EXAMPLE_DIR)/$$example test -v -race ./...; \
	done

lint:
	golangci-lint run ./...
	@set -e; for example in $(EXAMPLES); do \
		echo "lint examples/$$example"; \
		cd $(EXAMPLE_DIR)/$$example && golangci-lint run ./...; \
	done

nilaway: mod-tidy ## Run nilaway nil safety analysis
	go run go.uber.org/nilaway/cmd/nilaway@latest $(NILAWAY_FLAGS) ./...

# Build example binaries
# Depends on source and module files to determine when rebuild is needed
.SECONDEXPANSION:
$(EXAMPLES): $(GO_FILES) $(GO_MOD_FILES) \
	$$(EXAMPLE_DIR)/$$@/go.mod \
	$$(EXAMPLE_DIR)/$$@/go.sum
	go -C $(EXAMPLE_DIR)/$(@) build -o $(ROOT_DIR)/$(@) .
