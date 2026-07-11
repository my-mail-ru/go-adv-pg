GO = go
GOSRC = $(shell find . -type f -name '*.go')

TEST_TIMEOUT = 1m
LINT_TIMEOUT = 5m

.PHONY: all
all: format deps test full-lint doc

.PHONY: deps
deps:
	$(GO) mod tidy -v && $(GO) mod verify

.PHONY: lint
lint:
	golangci-lint run --timeout=$(LINT_TIMEOUT) --new-from-rev=origin/$(shell (grep -s DEFAULT_BRANCH: .gitlab-ci.yml | cut -d: -f2 | sed 's/^\s*$$/master/' | tr -d ' \t'; echo master) | head -n1) ./...
	golangci-lint run --timeout=$(LINT_TIMEOUT) --build-tags=integration --new-from-rev=origin/$(shell (grep -s DEFAULT_BRANCH: .gitlab-ci.yml | cut -d: -f2 | sed 's/^\s*$$/master/' | tr -d ' \t'; echo master) | head -n1) ./...

.PHONY: full-lint
full-lint:
	golangci-lint run ./...
	golangci-lint run --build-tags=integration ./...

.PHONY: format
format:
	find . -name '*.go' | grep -v /vendor/ | xargs goimports -local github.com/my-mail-ru/ -l -w

.PHONY: test
test:
	$(GO) test $(GOFLAGS) -v     \
	    -timeout $(TEST_TIMEOUT) \
	    -race                    \
	    -covermode atomic        \
	    -coverprofile cover.out  \
	    -coverpkg ./...          \
	    ./...
	$(GO) tool cover -html cover.out -o cover.html

.PHONY: test-integration
test-integration: test-conf dev-env-start
	$(GO) test $(GOFLAGS) -v                \
	    -count=1                            \
	    -tags=integration                   \
	    -race                               \
	    -covermode atomic                   \
	    -coverprofile cover.integration.out \
	    -coverpkg ./...                     \
	    ./...                            && \
	$(GO) tool cover -html cover.integration.out -o cover.integration.html

.PHONY: test-conf
test-conf: internal/test/testdata/config.cdb

%.cdb: %.yaml
	$(GO) tool yaml2cdb -in $< -out $@

dev-env-start:
	docker compose -f internal/test/testdata/docker-compose.yaml up -d
	docker compose -f internal/test/testdata/docker-compose.yaml wait create-tables

dev-env-stop:
	docker compose -f internal/test/testdata/docker-compose.yaml down

.PHONY: doc
doc: docs/DOCUMENTATION.md

.PHONY: generate
generate:
	go generate ./...

docs/DOCUMENTATION.md: $(GOSRC)
	gomarkdoc --tags gomarkdoc `ls -d ./internal/*|sed 's/^/--exclude-dirs /'` --output $@ ./...
