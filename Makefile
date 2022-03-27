.PHONY: clean api-builder api

USER := $(shell whoami)
OS := $(shell uname)

#####
# DEV
#####
.PHONY: infra infra-stop
infra:
	./scripts/start_development.sh

infra-stop:
	./scripts/stop_development.sh

#####
# API
#####
api:
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
