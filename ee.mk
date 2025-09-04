# ee.mk — private build battery for the tagged (-tags mailez_ee) tree.
#
# Never exported to the public source tree. Use with:
#   make -f ee.mk verify-ee
#
# The purity gate is the contract that keeps the public export clean:
#   1. the no-tag dependency graph must never reach backend/internal/ee;
#   2. every mailez_ee-tagged file must live under internal/ee/ or match
#      the *_ee.go / *_ee_test.go naming the export strips;
#   3. no file named *enterprise* may sit outside those patterns;
#   4. the export itself must pass its content-level post-conditions
#      (export-ce-check below).

TAGS = -tags mailez_ee

.PHONY: verify-ee vet-ee test-ee build-ee purity export-ce-check export-ce

## verify-ee: the full battery for the tagged tree
verify-ee: purity vet-ee test-ee build-ee

vet-ee:
	cd backend && go vet $(TAGS) ./...

test-ee:
	cd backend && go test $(TAGS) ./... -count=1

build-ee:
	cd backend && go build $(TAGS) ./...

purity:
	@cd backend; \
	deps=$$(go list -deps ./... 2>/dev/null | grep -c 'mailez/backend/internal/ee'); \
	if [ "$$deps" != "0" ]; then \
		echo "untagged build reaches backend/internal/ee ($$deps packages) — forbidden"; \
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
	echo "purity: ok"

## export-ce-check: run the export without pushing and fail on any
## post-condition violation (content-level scan included).
export-ce-check:
	MAILEZ_EXPORT_CHECK=1 scripts/export-ce.sh

## export-ce: regenerate and push the public tree (needs MAILEZ_PUBLIC_URL)
export-ce:
	scripts/export-ce.sh
