.PHONY: help changelog-check check-commits doc-links use-cases install install-go install-frontend dev seed format format-check lint lint-go lint-frontend typecheck i18n-check test test-scripts test-go test-frontend test-coverage test-coverage-go test-coverage-frontend test-carddav test-e2e test-e2e-docker arch openapi openapi-check vuln-go audit-frontend fuzz precommit build clean clean-all

help: ## Show this help message
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-24s %s\n", $$1, $$2}'

install: install-go install-frontend ## Install backend and frontend dependencies

install-go: ## Download Go modules
	cd backend && go mod download

install-frontend: frontend/node_modules/.package-lock.json ## Install frontend dependencies from the lockfile

# npm ci always reinstalls from scratch; npm writes this file, so it runs only after a lockfile change
frontend/node_modules/.package-lock.json: frontend/package.json frontend/package-lock.json
	cd frontend && npm ci

dev: install ## Run backend and frontend dev servers (no setup; optional overrides in dev.env)
	./scripts/dev.sh

seed: ## Add a demo user (demo / DemoPassword123!) and contacts to the running "make dev" backend
	set -a; [ ! -f dev.env ] || . ./dev.env; set +a; node scripts/seed.mjs

PRETTIER = frontend/node_modules/.bin/prettier

format: install-frontend ## gofmt and Prettier (TS, JSON, YAML, Markdown, ...) on all files
	cd backend && gofmt -w .
	$(PRETTIER) --write --log-level warn .

format-check: install-frontend ## Fail if any file is not gofmt/Prettier-clean
	@bad=$$(cd backend && gofmt -l .); \
	if [ -n "$$bad" ]; then echo "Not gofmt-clean:"; echo "$$bad"; exit 1; fi
	$(PRETTIER) --check --log-level warn .

lint: lint-go lint-frontend doc-links changelog-check ## go vet + golangci-lint + eslint + Markdown link and changelog checks

lint-go: install-go ## go vet + golangci-lint (the golangci-lint binary must be installed)
	cd backend && go vet ./...
	cd backend && golangci-lint run

lint-frontend: install-frontend ## ESLint
	cd frontend && npx eslint --max-warnings=0

typecheck: install-frontend ## TypeScript type check (app and E2E/Playwright config)
	cd frontend && npx tsc --noEmit
	cd frontend && npx tsc -p tsconfig.node.json --noEmit

i18n-check: ## Locale files: missing/extra keys fail, unused/duplicate keys warn (scripts/i18n-check.mjs)
	node scripts/i18n-check.mjs

test-scripts: ## Tests of the helper scripts in scripts/
	node --test scripts/*.test.mjs

changelog-check: ## Check the [Unreleased] entries of CHANGELOG.md against the AGENTS.md rules
	node scripts/check-changelog.mjs

check-commits: ## Check commit subjects of a range (make check-commits RANGE=origin/omk..HEAD)
	node scripts/check-commits.mjs $(RANGE)

doc-links: ## Check repo-relative Markdown links and anchors (offline)
	node scripts/check-doc-links.mjs

use-cases: ## Check the use-case catalogue against the tests; lists the uncovered use cases
	node scripts/use-cases.mjs

GO_TEST_FLAGS ?=
# Minimum total Go coverage in percent (current value rounded down). Raise it when coverage rises.
GO_COVERAGE_MIN ?= 82

test: test-scripts use-cases test-go test-frontend ## Backend, frontend and script unit tests, and the use-case check

test-go: install-go ## Go tests (GO_TEST_FLAGS="-race" adds flags)
	cd backend && go test $(GO_TEST_FLAGS) ./...

test-frontend: install-frontend ## Vitest
	cd frontend && npm test

test-coverage: test-coverage-go test-coverage-frontend ## Tests with coverage (backend coverage.out, frontend coverage/)

test-coverage-go: install-go ## Go tests with coverage (backend/coverage.out), fail below GO_COVERAGE_MIN
	cd backend && go test $(GO_TEST_FLAGS) -coverpkg=./... -coverprofile=coverage.out ./...
	cd backend && node ../scripts/check-go-coverage.mjs $(GO_COVERAGE_MIN)

test-coverage-frontend: install-frontend ## Vitest with coverage (frontend/coverage/), fail below the thresholds in vite.config.ts
	cd frontend && npm run test:coverage

test-carddav: install-go ## Sync tests against a running Radicale (see docs/development/testing.md)
	cd backend && MEERKAT_CARDDAV_IT=1 go test ./services/ -v

test-e2e: install ## Playwright E2E against a local stack (no Docker; see scripts/e2e-stack.mjs)
	node scripts/e2e-stack.mjs -- npx playwright test $(ARGS)

test-e2e-docker: install-frontend ## Playwright against an already running stack on :7300 (CI: all-in-one image)
	cd frontend && npx playwright test $(ARGS)

openapi: install ## Write the OpenAPI 3.1 document (docs/openapi.json) and the TypeScript types generated from it
	cd backend && go run ./cmd/openapi ../docs/openapi.json
	cd frontend && npx openapi-typescript ../docs/openapi.json -o src/types/generated/api.ts

openapi-check: install ## Fail when docs/openapi.json or the generated TypeScript types are out of date (run make openapi)
	@d=$$(mktemp -d) && trap 'rm -rf "$$d"' EXIT && \
	(cd backend && go run ./cmd/openapi "$$d/openapi.json") && \
	(cd frontend && npx openapi-typescript "$$d/openapi.json" -o "$$d/api.ts" >/dev/null) && \
	diff -u docs/openapi.json "$$d/openapi.json" && diff -u frontend/src/types/generated/api.ts "$$d/api.ts" \
	|| { echo "openapi: generated files are out of date. Run: make openapi"; exit 1; }

vuln-go: install-go ## govulncheck: fail on reachable Go vulnerabilities
	cd backend && go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

audit-frontend: ## npm audit of production dependencies, high and above
	cd frontend && npm audit --omit=dev --audit-level=high

FUZZ_PACKAGE ?= ./services
FUZZ_TARGET ?= FuzzCSVImport
FUZZ_TIME ?= 30s

fuzz: install-go ## Fuzz one target: make fuzz FUZZ_PACKAGE=./services FUZZ_TARGET=FuzzCSVImport FUZZ_TIME=30s
	cd backend && go test "$(FUZZ_PACKAGE)" -run '^$$' -fuzz "^$(FUZZ_TARGET)\$$" -fuzztime "$(FUZZ_TIME)"

arch: install ## Architecture rules: Go depguard layering, frontend dependency-cruiser (docs/adr/0010-target-layering.md)
	cd backend && golangci-lint run --enable-only depguard
	node scripts/arch-frontend.mjs

precommit: format-check lint typecheck i18n-check arch openapi-check test-scripts use-cases test-coverage test-e2e ## All checks, including E2E

build: install ## Build backend binary and frontend bundle
	cd backend && go build -o meerkat .
	cd frontend && VITE_API_URL= npm run build

clean: ## Remove build outputs, coverage, test reports and the make dev data
	rm -rf .dev
	rm -rf backend/coverage.out
	rm -rf backend/meerkat
	rm -rf frontend/build
	rm -rf frontend/coverage
	rm -rf frontend/dist
	rm -rf frontend/playwright-report
	rm -rf frontend/test-results

clean-all: clean ## clean plus frontend/node_modules
	rm -rf frontend/node_modules
