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

registry:
	go run main.go registry -r 50000

event_store:
	go run main.go event_store -r 50001

local:
	make -j 2 registry event_store

test:
	cd tests/registry && go run main.go 

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
	docker build -t apibuilder:v1 -f ./Dockerfile.compiler .;

# cleanup generated code
clean:
	sudo rm -rf ./api/gen/
