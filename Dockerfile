# builds on the host platform and cross compiles, fast for multi-arch images
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS builder

RUN apk add --no-cache ca-certificates
WORKDIR /src
ENV CGO_ENABLED=0

# cache modules
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /ptt-alertor .

# about 15MB: the binary with embedded time zones, and certificates
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /ptt-alertor /ptt-alertor

ENV TZ=Asia/Taipei
EXPOSE 9090
HEALTHCHECK --interval=1m --timeout=10s --start-period=30s CMD ["/ptt-alertor", "healthcheck"]
ENTRYPOINT ["/ptt-alertor"]
