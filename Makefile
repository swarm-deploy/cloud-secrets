.PHONY: test
test:
	docker stack deploy -c docker-compose.yaml cloud-secrets --detach=false

.PHONY: lint
lint:
	golangci-lint run

.PHONY: build
build:
	docker build . -t swarmdeployorg/cloud-secrets:local
