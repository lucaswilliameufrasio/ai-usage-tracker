.PHONY: quality fmt vet test web icons verify-ui

quality: fmt vet test web

fmt:
	@test -z "$$(gofmt -l .)"

vet:
	go vet ./...

test:
	go test ./...

icons:
	python3 web/scripts/make-icons.py

web: icons
	cd web && pnpm install --frozen-lockfile && pnpm test && pnpm build

verify-ui: web
	cd web && node --experimental-strip-types scripts/verify-journey.ts
