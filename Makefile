.PHONY: test
test:
	go test ./...
	docker stack deploy -c docker-compose.local.yaml cloud-secrets --detach=false

.PHONY: lint
lint:
	golangci-lint run

.PHONY: build
build:
	docker build . -t swarmdeployorg/cloud-secrets:local
