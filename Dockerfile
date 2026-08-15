# syntax=docker/dockerfile:1

FROM golang:1.26.2-alpine AS builder

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

LABEL org.opencontainers.image.title="cloud-secrets"
LABEL org.opencontainers.image.description="Background service for update secrets in Docker Swarm cluster"
LABEL org.opencontainers.image.url="https://github.com/swarm-deploy/cloud-secrets"
LABEL org.opencontainers.image.source="https://github.com/swarm-deploy/cloud-secrets"
LABEL org.opencontainers.image.vendor="swarm-deploy"
LABEL org.opencontainers.image.version="$APP_VERSION"
LABEL org.opencontainers.image.created="$BUILD_TIME"
LABEL org.opencontainers.image.licenses="Apache 2.0"
LABEL org.swarm-deploy.service.type="SecretManager"

ENTRYPOINT ["/go/bin/cloud-secrets"]
