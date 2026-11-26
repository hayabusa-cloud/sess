.PHONY: check test bench

check:
	@files="$$(gofmt -l .)" && test -z "$$files" || { printf '%s\n' "$$files"; exit 1; }
	go mod tidy -diff
	go vet ./...

test:
	go test -timeout 120s -count=1 ./...
	go test -timeout 120s -race -cover ./...

bench:
	go test -mod=readonly -run '^$$' -bench=. -benchmem -benchtime=200ms -count=10 -timeout=5m .
