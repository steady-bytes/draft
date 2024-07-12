## dctl infra

Manage all local draft infra services (Docker containers)

### Synopsis

Manage all local draft infra services (Docker containers). dctl runs all local
infrastructure as Docker containers and you can manage their lifecycle with the commands
below this one.

Note that you can specify which services to operate on with any of the other commands
using the flag --services:

dctl infra start --services 'postgres,nats,hasura'

### Options

```
  -h, --help   help for infra
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl](dctl.md)	 - dctl (Draft Controller) is the built-in CLI for managing everything in Draft
* [dctl infra clean](dctl_infra_clean.md)	 - Clean up infra resources (Docker containers)
* [dctl infra init](dctl_infra_init.md)	 - Pull Docker images required for draft infra
* [dctl infra start](dctl_infra_start.md)	 - Run draft infra Docker containers
* [dctl infra status](dctl_infra_status.md)	 - Check the status of all local infra
* [dctl infra stop](dctl_infra_stop.md)	 - Stop running draft infra Docker containers

