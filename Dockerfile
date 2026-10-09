# building binary
FROM golang:1.15-alpine as builder

ENV GOPATH /go/
ENV GO_WORKDIR $GOPATH/src/github.com/Ptt-Alertor/ptt-alertor/
ENV GO111MODULE=on
ENV CGO_ENABLED=0

WORKDIR $GO_WORKDIR

# cache modules
COPY go.mod go.sum $GO_WORKDIR
RUN go mod download

ADD . $GO_WORKDIR

RUN go install

# building executable image
FROM alpine:3.20

RUN set -eux; \
	apk add --no-cache ca-certificates tzdata

COPY public/ public/
COPY --from=builder /go/bin/ptt-alertor .

ENTRYPOINT ["/ptt-alertor"]

EXPOSE 9090 6060
