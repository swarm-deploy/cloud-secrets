.PHONY: gen
gen:
	protoc -I api \
		--go_out=. --go_opt=module=github.com/swarm-deploy/cloud-secrets \
		--go-grpc_out=. --go-grpc_opt=module=github.com/swarm-deploy/cloud-secrets \
		$$(find api -type f -name '*.proto' | sort)
	go generate ./...

.PHONY: test/cloudru
test/cloudru:
	docker stack deploy -c tests/cloudru.yaml cloud-secrets-cloudru --detach=false

.PHONY: test/vault
test/vault:
	docker stack deploy -c tests/vault.yaml cloud-secrets-vault --detach=false

.PHONY: test
test:
	go test ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: build
build:
	docker build . -t swarmdeployorg/cloud-secrets:local

.PHONY: e2e
e2e:
	docker build . -t swarmdeployorg/cloud-secrets:local
	CS_E2E=1 go test ./e2e/...
