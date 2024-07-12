## dctl context import

Import an existing context

### Synopsis

Import an existing repository as a draft context for dctl to manage. The
repository must have a valid draft workspace config file at its root (draft.yaml).

```
dctl context import [flags]
```

### Examples

```
dctl context import --path /home/userA/repos/repoX
```

### Options

```
  -h, --help          help for import
  -p, --path string   the path to import the context from (default ".")
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl context](dctl_context.md)	 - Commands to manage draft contexts

