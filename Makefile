.PHONY: build build-version build-demo lint lint-fix lint-docs vet test clean check check-demo tidy fix tag preview e2e e2e-gen e2e-update nix-deps

# The version comes from the git tag: v0.1.0 on a tag, v0.1.0-3-gabc1234 three
# commits after it, abc1234 when there is no tag yet. Override with VERSION=...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
LDFLAGS = -X main.version=$(or $(VERSION),dev)

build:
	go build -ldflags "$(LDFLAGS)" -o lazybitbucket ./cmd/lazybitbucket

build-demo:
	go build -tags demo -ldflags "$(LDFLAGS)" -o lazybitbucket ./cmd/lazybitbucket

# Kept for the old name: build stamps the version too now.
build-version: build

lint:
	go tool golangci-lint run ./...

lint-fix:
	go tool golangci-lint fmt
	go tool golangci-lint run --fix ./...

lint-docs:
	npx --yes markdownlint-cli README.md CHANGELOG.md docs/*.md --disable MD001 MD013 MD024 MD033 MD040 MD041 MD060

vet:
	go vet ./...

test:
	go test -race ./...

clean:
	rm -f lazybitbucket

check: lint vet build test

tidy:
	go mod tidy

fix: tidy lint-fix nix-deps

# Tag a version: make tag V=0.2.0. Nothing is published; the tag only sets the
# version that build stamps. CHANGELOG.md must have the section first, so move
# the [Unreleased] entries under "## [0.2.0] - <date>" and commit before this.
# push.followTags sends an annotated tag with the next push of the branch.
tag:
	@test -n "$(V)" || (echo "Usage: make tag V=0.2.0" && exit 1)
	@grep -q '^## \[$(V)\]' CHANGELOG.md || (echo "CHANGELOG.md has no [$(V)] section" && exit 1)
	@test -z "$$(git status --porcelain)" || (echo "the tree is not clean: commit first" && exit 1)
	git tag -a v$(V) -m v$(V)
	@echo "Tagged v$(V). It goes out with the next push of the branch."

check-demo:
	go tool golangci-lint run --build-tags demo ./...
	go vet -tags demo ./...
	go test -race -tags demo ./cmd/...
	go build -tags demo -o lazybitbucket ./cmd/lazybitbucket

preview: build-demo e2e-gen
	@vhs -q e2e/tapes/00_preview.tape & vhs -q e2e/tapes/00_preview_vertical.tape & wait
	@rm -f e2e/tapes/*.tape

e2e: build-demo e2e-gen
	@pids=""; fail=0; \
	for tape in e2e/tapes/*.tape; do \
		echo "Running $$tape..."; \
		vhs -q $$tape & pids="$$pids $$!"; \
	done; \
	for pid in $$pids; do \
		wait $$pid || fail=1; \
	done; \
	if [ $$fail -eq 1 ]; then echo "SOME TAPES FAILED" && exit 1; fi
	@rm -f e2e/tapes/*.tape
	@echo "All tapes passed."

e2e-gen:
	@./e2e/tape.sh generate-all

nix-deps:
	gomod2nix generate

e2e-update: build-demo e2e-gen
	@pids=""; \
	for tape in e2e/tapes/*.tape; do \
		echo "Running $$tape..."; \
		vhs -q $$tape & pids="$$pids $$!"; \
	done; \
	for pid in $$pids; do wait $$pid; done
	@rm -f e2e/tapes/*.tape
	@echo "Golden files updated. Review with: git diff e2e/golden/"
