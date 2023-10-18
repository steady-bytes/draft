# API
To serve as the home for type definitions, rpc service's, model's, validation of messages, and gateway generation.
All of the above are declared in [protocol buffer's]() and compiled using [buf]() inside of a docker container
to reduce protoc compiler, and compiler plugin environment dependencies.

# service-clients-internal

Contains all Fluid Truck defined protobuf files.

## Generating files for local testing

### Using `fctl`

The easiest way to generate protos is using [`fctl`](https://github.com/fluidtruck/fctl):

```shell
# you only need to run this the first time
fctl api init
fctl api build
```

Any time you want to regenerate your protos just run `fctl api build` again.

### Manually

`make install` needs to be run only once to install required go modules.

There are two targets to compiling code locally:

- `make go` will compile all protobuf files into go and place them in the `gen/go` directory.
- `make go-diff` will compile only added & changed protobuf files into go and place them in the `gen/go` directory.

To use these generated files locally use this `replace` in the project's go.mod:

`replace github.com/fluidtruck/service-clients-internal => ../protobufs/gen/go`

git will ignore all generated files.

The manual work flow becomes:

1. pull main & checkout new branch
2. `make go`
3. add `replace` to project's go.mod
4. add and modify protos
5. `make go-diff`

If a `.proto` is deleted, `make go` will be required.

Using `make go` exclusively works but `make go-diff` will be faster for normal development.

## Using locally generated files

To use the generated files locally, you will need to add a `replace` to the project's go.mod:

```go.mod
...

replace github.com/fluidtruck/service-clients-internal => ../protobufs/gen/go

...
```

## Using a gRPC GUI

### Postman

To use Postman, you can simply use gRPC reflection to "import" the proto defintions. There are two ways to do this depending on if you are running the [proxy](https://github.com/fluidtruck/proxy) or not.

If you are using the proxy, for the URL in Postman you can set it to `localhost:10000` + the path of *any* gRPC service that is implemented by the microservice you wish to test. For example, if you are testing the [users](https://github.com/fluidtruck/users) microservice, it implements a few different gRPC services including `users.v1.PointsOfContactService` and `users.v1.UserPermissionsService`. To use reflection, simply choose *any* of the implemented services and use that as your URL. For the `users` microservice example we can use `localhost:10000/users.v1.UserPermissionsService` and this will pull in *all* gRPC services implemented by the `users` microservice.

If you are not using the proxy, for the URL in Postman you will need to find the exact port of the service you are testing. When you run the service it will log this out to the console. For example, if you are testing a service that is running on port `54879`, you will need to set your Postman URL to `localhost:54879`. This will also pull in *all* gRPC services implemented by the microservice.

### BloomRPC

To use BloomRPC, you will need to import the top-level of the `protobufs` repo as well as any vendor submodules that are used in the proto files you will be testing. The imports should look like this:

```text
<USER PATH TO GIT REPOS>/protobufs
<USER PATH TO GIT REPOS>/protobufs/vendor/googleapis
<USER PATH TO GIT REPOS>/protobufs/vendor/grpc-gateway
<USER PATH TO GIT REPOS>/protobufs/vendor/protoc-gen-gotag
<USER PATH TO GIT REPOS>/protobufs/vendor/protoc-gen-validate
...
```

Then you can import individual proto files into the GUI and test them.

NOTE: if you did not originally clone the repository with git submodules, you will need to run `git submodule update --init --recursive` to pull in the submodules.
