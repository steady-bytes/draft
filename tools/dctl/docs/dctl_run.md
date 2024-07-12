## dctl run

Run services locally

### Synopsis

Run services locally. You can either run specific services using the --services (-s)
flag or run entire domains using the --domains (-d) flag.

For example, to run just the 'echo' service within the 'examples' domain you could do:

dctl run -s examples/echo

And to run all services within both the 'examples' domain and the 'core' domain you could do:

dctl run -d examples,core


```
dctl run [flags]
```

### Options

```
  -d, --domains strings    domain(s) to run (e.g. 'core' or 'core,examples')
  -h, --help               help for run
  -s, --services strings   service(s) to run (e.g. 'core/blueprint' or 'core/blueprint,core/fuse')
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl](dctl.md)	 - dctl (Draft Controller) is the built-in CLI for managing everything in Draft

