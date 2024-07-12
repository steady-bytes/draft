## dctl api build

Generate code from all protobuf files

### Synopsis

Generate code from all protobuf files. Make sure to run 'dctl api init' before running this as
this command requires the setup performed by init.

Note that you can override the image defined by the workspace config with the --image flag:

dctl api build --image draft-proto-builder:local

```
dctl api build [flags]
```

### Examples

```
dctl api build
```

### Options

```
  -h, --help           help for build
  -i, --image string   override the builder image name from the workspace config
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl api](dctl_api.md)	 - Commands for managing the draft API (protobufs)

