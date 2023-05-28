FROM golang:1.20-alpine AS builder
# Install git: required for fetching the Go dependencies.
RUN apk update && apk add --no-cache git
ENV GOPROXY=direct GOSUMDB=off
WORKDIR /go/src/project
ADD . .
RUN env CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o vega-exporter .

FROM alpine:3
COPY --from=builder /go/src/project/vega-exporter /
ENTRYPOINT ["/vega-exporter"]
