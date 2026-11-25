.PHONY: check test bench

check:
	gofmt -l . | grep . && exit 1 || true
	go mod tidy
	go vet ./...

test:
	go test -timeout 120s -count=1 ./...
	go test -timeout 120s -race -cover ./...

bench:
	go test -mod=readonly -run '^$$' -bench=. -benchmem -benchtime=200ms -count=10 -timeout=5m .
