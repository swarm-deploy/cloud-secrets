# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine AS builder

ARG APP_VERSION="undefined"
ARG BUILD_TIME="undefined"

WORKDIR /go/src/github.com/artarts36/cloud-secrets

RUN apk add git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w -X 'main.Version=${APP_VERSION}' -X 'main.BuildDate=${BUILD_TIME}'" -o /go/bin/cloud-secrets /go/src/github.com/artarts36/cloud-secrets/cmd/cloud-secrets/main.go

######################################################

FROM scratch

COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

COPY --from=builder /go/bin/cloud-secrets /go/bin/cloud-secrets

EXPOSE 8000

ENTRYPOINT ["/go/bin/cloud-secrets"]
