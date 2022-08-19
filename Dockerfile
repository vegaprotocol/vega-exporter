FROM golang:1.18-alpine AS builder
# Install git: required for fetching the Go dependencies.
RUN apk update && apk add --no-cache git
ENV GOPROXY=direct GOSUMDB=off
WORKDIR /go/src/project
ADD . .
RUN env CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o vega_exporter .

FROM scratch
COPY --from=builder /go/src/project/vegatools /
ENTRYPOINT ["/vega_exporter"]
