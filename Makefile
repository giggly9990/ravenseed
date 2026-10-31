TEST_STACK = docker compose -p ravenseed-demo -f docker-compose.yml -f docker-compose.demo.yml

.PHONY: test test-race fmt-check smoke test-stack-down build up down

# Installs web dependencies only when package-lock.json changed.
web/node_modules/.package-lock.json: web/package-lock.json
	cd web && npm ci

test: web/node_modules/.package-lock.json
	go vet ./...
	go test ./...
	cd web && npm run typecheck && npm test && npm run build

# The race detector needs cgo (a C compiler on PATH).
test-race:
	CGO_ENABLED=1 go test -race ./...

fmt-check: web/node_modules/.package-lock.json
	@files="$$(gofmt -l cmd internal)"; if [ -n "$$files" ]; then echo "gofmt needed:"; echo "$$files"; exit 1; fi
	cd web && npm run format:check

# Starts the test stack, seeds it and runs the search smoke test.
# `make test-stack-down` removes it with its data.
smoke:
	$(TEST_STACK) up -d --build --wait --wait-timeout 300
	MOCK_QUI_URL=http://mock-qui:7476 node scripts/demo-seed.mjs http://localhost:8090
	node scripts/smoke.mjs http://localhost:8090

test-stack-down:
	$(TEST_STACK) down -v

build:
	docker compose build
up:
	docker compose up -d --build
down:
	docker compose down
