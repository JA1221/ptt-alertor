FROM golang:1.23-alpine AS builder

WORKDIR /src
ENV CGO_ENABLED=0

# cache modules
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /ptt-alertor .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /ptt-alertor /ptt-alertor

EXPOSE 9090
ENTRYPOINT ["/ptt-alertor"]
