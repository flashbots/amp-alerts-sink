VERSION := $(shell git describe --tags --always --dirty="-dev" --match "v*.*.*" || echo "development" )
VERSION := $(VERSION:v%=%)

.PHONY: build
build:
	@CGO_ENABLED=0 go build \
			-ldflags "-X main.version=${VERSION}" \
			-o ./bin/amp-alerts-sink \
		github.com/flashbots/amp-alerts-sink/cmd

.PHONY: docker
docker:
	docker build \
			--build-arg VERSION=${VERSION} \
			--platform linux/amd64 \
			--tag amp-alerts-sink/cmd:${VERSION} \
		.
	@echo ""
	@echo "Built image: amp-alerts-sink/cmd:${VERSION}"

.PHONY: mockgen
mockgen:
	@mockgen \
			-destination mock/db/db.go \
			-package mock_db \
		github.com/flashbots/amp-alerts-sink/db DB

	@mockgen \
			-destination mock/publisher/slack.go \
			-package mock_publisher \
		github.com/flashbots/amp-alerts-sink/publisher SlackApi

.PHONY: snapshot
snapshot:
	@goreleaser release --snapshot --clean

.PHONY: test
test:
	@go test ./...

.PHONY: help
help:
	@go run github.com/flashbots/amp-alerts-sink/cmd lambda --help

.PHONY: lambda
lambda:
	@go run github.com/flashbots/amp-alerts-sink/cmd lambda
