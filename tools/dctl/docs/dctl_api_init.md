## dctl api init

Initialize a draft workspace for generating code from protos

### Synopsis

Initialize a draft workspace for generating code from protos. This involves
pulling or building the protobuf builder Docker image and installing Node modules.

By default dctl will try and pull the Docker image defined in the draft workspace config but you can tell it
to build the image locally instead:

dctl api init --build true --image draft-proto-builder:local

Be sure to include the --image flag on any subsequent calls to build:

dctl api build --image draft-proto-builder:local

```
dctl api init [flags]
```

### Examples

```
dctl api init
```

### Options

```
  -b, --build          build the Docker image instead of pulling it
  -h, --help           help for init
  -i, --image string   required when --build=true. defines the name of the image to build
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl api](dctl_api.md)	 - Commands for managing the draft API (protobufs)

