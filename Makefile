.PHONY: backend-verify backend-verify-ee frontend-verify verify verify-ee e2e images check-ce-purity format fmt-backend fmt-frontend

## backend-verify: gofmt / vet / tidy / tests for the Go backend
backend-verify:
	cd backend && test -z "$$(gofmt -l .)" && go vet ./... && go mod tidy -diff && go test ./... -count=1

## backend-verify-ee: the same battery with -tags mailez_ee
backend-verify-ee:
	cd backend && go vet -tags mailez_ee ./... && go test -tags mailez_ee ./... -count=1

## frontend-verify: typecheck / lint / tests for both Next.js apps
frontend-verify:
	cd frontend/apps/webmail && npm run typecheck && npm run lint && npm test
	cd frontend/apps/admin && npm run typecheck && npm run lint && npm test

## verify: run every check the CI runs
verify: backend-verify frontend-verify

## verify-ee: enterprise edition battery (run in the private CI only)
verify-ee: backend-verify-ee

## e2e: end-to-end smoke against a running stack (dev compose + backend on 8080)
e2e:
	cd backend && go run ./cmd/e2e -api-port 8080 -smtp-port 25 -imap-port 143 --domain e2e.example.com --alias team

## images: build all mail images (shared + engines) with the Go builder
images:
	cd backend && go run ./cmd/build-images

## check-ce-purity: the community (no-tag) backend dependency graph must
## never reach backend/internal/ee, every EE-tagged file must match
## the export strip contract (internal/ee/ or *_ee.go / *_ee_test.go), and
## enterprise-named files must carry an _ee/_ce suffix so untagged
## enterprise code cannot ride along in the community build.
check-ce-purity:
	cd backend && deps=$$(go list -deps ./... 2>/dev/null | grep -c 'mailez/backend/internal/ee'); \
	if [ "$$deps" != "0" ]; then \
		echo "CE build reaches backend/internal/ee ($$deps packages) — forbidden"; \
		go list -deps ./... | grep 'mailez/backend/internal/ee'; \
		exit 1; \
	fi; \
	bad=$$(grep -rlE '^//go:build mailez_ee' --include='*.go' --exclude-dir=.git . | grep -vE '/internal/ee/' | grep -vE '_ee\.go$$|_ee_test\.go$$'); \
	if [ -n "$$bad" ]; then \
		echo "EE-tagged files outside the export strip contract (rename with an _ee suffix):"; \
		echo "$$bad"; \
		exit 1; \
	fi; \
	namebad=$$(find . -name '*enterprise*.go' -not -path './internal/ee/*' | grep -vE '_ee(_test)?\.go$$|_ce(_test)?\.go$$'); \
	if [ -n "$$namebad" ]; then \
		echo "enterprise-named files outside the export strip contract (split with _ee/_ce suffixes):"; \
		echo "$$namebad"; \
		exit 1; \
	fi; \
	echo "ce-purity: ok"

fmt-backend:
	cd backend && gofmt -w .

fmt-frontend:
	cd frontend/apps/webmail && npm run format
	cd frontend/apps/admin && npm run format

## format: normalize formatting across backend and frontends
format: fmt-backend fmt-frontend
