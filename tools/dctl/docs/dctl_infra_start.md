## dctl infra start

Run draft infra Docker containers

```
dctl infra start [flags]
```

### Options

```
  -f, --follow             whether or not to follow the output of the infra docker containers (true/FALSE)
  -h, --help               help for start
  -s, --services strings   infra services to act on (default [nats,postgres])
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl infra](dctl_infra.md)	 - Manage all local draft infra services (Docker containers)

