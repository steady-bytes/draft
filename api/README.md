# API
To serve as the home for type definitions, rpc service's, model's, validation of messages, and gateway generation.
All of the above are declared in [protocol buffer's]() and compiled using [buf]() inside of a docker container
to reduce protoc compiler, and compiler plugin environment dependencies.

## Generating files for local testing

### Using `dctl`

The easiest way to generate protos is using [`dctl`](../tools/dctl/):

```shell
dctl api init
dctl api build
```

Any time you want to regenerate your protos just run `dctl api build` again.

## Using locally generated files

To use the generated files locally, you will need to add a `replace` to the project's go.mod:

```go.mod
...

replace github.com/steady-bytes/draft/api => ../../../api

...
```
