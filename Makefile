.PHONY: clean

USER := $(shell whoami)
OS := $(shell uname)

#####
# DEV
#####
.PHONY: start local

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
	BOOTSTRAP_RAFT=true RAFT_IP=localhost RAFT_PORT=1111 SERVER_PORT=2221 RAFT_NODE_ID="node_1" go run services/core/blueprint/main.go

blueprint_2:
	RAFT_PORT=1112 RAFT_IP=localhost SERVER_PORT=2222 RAFT_NODE_ID="node_2" go run services/core/blueprint/main.go

blueprint_3:
	RAFT_PORT=1113 SERVER_PORT=2223 RAFT_IP=localhost RAFT_NODE_ID="node_3" go run services/core/blueprint/main.go

blueprint_register_leader:
	go run pkg/blueprint-client/main.go
