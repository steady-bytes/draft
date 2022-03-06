.PHONY: clean api-builder api

# cleanup generated code
clean:
	sudo rm -rf ./gen/

# build the compiler
compiler:
	docker build -t apibuilder:v1 -f ./Dockerfile .;

# generate the api
api:
	docker run --volume "$(PWD):/api" --workdir /api apibuilder:v1 mod update
	docker run --volume "$(PWD):/api" --workdir /api apibuilder:v1 generate
