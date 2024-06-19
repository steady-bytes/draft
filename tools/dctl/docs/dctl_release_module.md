## dctl release module

Release a Go module

### Synopsis

Release a Go module using a git tag. This will check the latest tag for the given module
and will ask how you would like to increment the semantic version (major.minor.patch). It will create
a git tag with the new version and push it to the git origin.

```
dctl release module [flags]
```

### Options

```
  -h, --help          help for module
  -p, --path string   path of Go module to release (e.g. pkg/chassis or tools/dctl)
```

### Options inherited from parent commands

```
      --config string   config file
```

### SEE ALSO

* [dctl release](dctl_release.md)	 - Commands for releasing Draft components

