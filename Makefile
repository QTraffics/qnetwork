.PHONY: fmt , fmt_install , lint , lint_install , test, generate
fmt:
#	@gofumpt -l -w .
#	@gofmt -s -w .
#	@gci write --custom-order -s standard -s "prefix(github.com/qtraffics/)" -s "default" .
	@GOOS=linux golangci-lint fmt

fmt_install:
	go install -v mvdan.cc/gofumpt@latest
	go install -v github.com/daixiang0/gci@latest

lint: fmt
	GOOS=linux golangci-lint run

lint_install:
	go install -v github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0

test:
	go test ./...

generate:
	go generate ./...