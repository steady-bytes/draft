## dctl context init

Initialize a new draft context (project)

### Synopsis

Initialize a new draft context (project).

Run this from the root of a new git repository or provide the path to the root of the repository
as a flag. This command will scaffold out the required directories and configuration files for
the new draft project and import it as a context for dctl to manage.

```
dctl context init [flags]
```

### Examples

```
dctl context init --path /path/to/new/git/repo
```

### Options

```
  -h, --help          help for init
  -p, --path string   the path to initialize the context in (default ".")
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl context](dctl_context.md)	 - Commands to manage draft contexts

