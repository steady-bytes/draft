#!/bin/bash

# run `buf mod update`
docker run --volume "$(PWD):/api" --workdir /api apibuilder:v1 mod update

# run `buf mod generate`
docker run --volume "$(PWD):/api" --workdir /api apibuilder:v1 generate
