.PHONY: clean api-builder api

USER := $(shell whoami)
OS := $(shell uname)

#####
# DEV
#####
.PHONY: infra infra-stop start local
infra:
	./scripts/start_development.sh

infra-stop:
	./scripts/stop_development.sh

registrar:
	go run main.go registrar -r 50000

eventer:
	go run main.go eventer -r 50001

local:
	make -j2 registrar eventer

test:
	cd tests/registry && go run main.go 

clean:
	rm -rf node_1 && rm -rf node_2 && rm -rf node_3

blueprint: clean_blueprint blueprint_1 blueprint_2 blueprint_3

blueprint_1:
	BOOTSTRAP_RAFT=true RAFT_IP=localhost RAFT_PORT=1111 SERVER_PORT=2221 RAFT_NODE_ID="node_1" go run internal/blueprint/main.go

blueprint_2:
	RAFT_PORT=1112 RAFT_IP=localhost SERVER_PORT=2222 RAFT_NODE_ID="node_2" go run internal/blueprint/main.go

blueprint_3:
	RAFT_PORT=1113 SERVER_PORT=2223 RAFT_IP=localhost RAFT_NODE_ID="node_3" go run internal/blueprint/main.go

blueprint_register_leader:
	go run pkg/blueprint-client/main.go


#####
# API
#####
api: compiler
	docker run --volume "$(PWD)/api:/api" --workdir /api apibuilder:v1 mod update
	docker run --volume "$(PWD)/api:/api" --workdir /api apibuilder:v1 generate
	@if [ "$(OS)" = "Linux" ]; then\
		sudo chown -R $(USER):$(USER) $(PWD);\
	fi

# build the compiler
compiler:
	docker build -t apibuilder:v1 -f ./api/Dockerfile.compiler .;