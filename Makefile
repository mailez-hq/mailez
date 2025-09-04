.PHONY: backend-verify frontend-verify verify e2e images format fmt-backend fmt-frontend

## backend-verify: gofmt / vet / tidy / tests for the Go backend
backend-verify:
	cd backend && test -z "$$(gofmt -l .)" && go vet ./... && go mod tidy -diff && go test ./... -count=1

## frontend-verify: typecheck / lint / tests for both Next.js apps
frontend-verify:
	cd frontend/apps/webmail && npm run typecheck && npm run lint && npm test
	cd frontend/apps/admin && npm run typecheck && npm run lint && npm test

## verify: run every check the CI runs
verify: backend-verify frontend-verify

## e2e: end-to-end smoke against a running stack (dev compose + backend on 8080)
e2e:
	cd backend && go run ./cmd/e2e -api-port 8080 -smtp-port 25 -imap-port 143 --domain e2e.example.com --alias team

## images: build all mail images via docker buildx bake (see docker-bake.hcl)
images:
	docker buildx bake default mailezine

fmt-backend:
	cd backend && gofmt -w .

fmt-frontend:
	cd frontend/apps/webmail && npm run format
	cd frontend/apps/admin && npm run format

## format: normalize formatting across backend and frontends
format: fmt-backend fmt-frontend
