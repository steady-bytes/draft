## dctl completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	dctl completion fish | source

To load completions for every new session, execute once:

	dctl completion fish > ~/.config/fish/completions/dctl.fish

You will need to start a new shell for this setup to take effect.


```
dctl completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --config string   config file
```

### SEE ALSO

* [dctl completion](dctl_completion.md)	 - Generate the autocompletion script for the specified shell

