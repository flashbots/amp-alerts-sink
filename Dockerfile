# stage: build ---------------------------------------------------------

FROM golang:1.22-bookworm AS build

# RUN apk add --no-cache gcc musl-dev linux-headers

WORKDIR /go/src/github.com/flashbots/amp-alerts-sink

COPY go.* ./
RUN go mod download

COPY . .

ARG VERSION=development

RUN go build \
        -ldflags "-s -w" \
        -ldflags "-X main.version=${VERSION}" \
        -o bin/amp-alerts-sink \
    github.com/flashbots/amp-alerts-sink/cmd

# stage: run -----------------------------------------------------------

FROM gcr.io/distroless/cc-debian12:nonroot

WORKDIR /app

COPY --from=build /go/src/github.com/flashbots/amp-alerts-sink/bin/amp-alerts-sink \
    ./amp-alerts-sink

ENTRYPOINT ["/app/amp-alerts-sink"]
CMD        ["lambda"]
