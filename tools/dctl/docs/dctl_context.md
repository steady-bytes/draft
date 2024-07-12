## dctl context

Commands to manage draft contexts

### Synopsis

Commands to manage draft contexts. A draft context defines a draft system for dctl to manage.

By default, dctl will choose a context similarly to how git does where the current working
directory informs context selection. If there is a draft workspace config (draft.yaml) within a parent directory that context will be used.
Otherwise dctl will use the default from the dctl config file (you can change this by calling 'dctl context set'). You can always
override the selected context by providing the --context flag on any dctl command.

Any repo with a draft workspace config file (draft.yaml) can be imported into dctl using the
'draft context import' command. You can also initializes a new draft project (context) using the
'draft context init' command.

### Options

```
  -h, --help   help for context
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl](dctl.md)	 - dctl (Draft Controller) is the built-in CLI for managing everything in Draft
* [dctl context import](dctl_context_import.md)	 - Import an existing context
* [dctl context init](dctl_context_init.md)	 - Initialize a new draft context (project)
* [dctl context set](dctl_context_set.md)	 - Set the default context

