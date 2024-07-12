## dctl pipelines run

Run a pipeline

```
dctl pipelines run [flags]
```

### Options

```
  -d, --directory string   the directory within the repo to test
  -h, --help               help for run
  -p, --pipeline string    the pipeline to run
  -r, --repo string        the git repository url to clone (default "git@github.com:steady-bytes/draft.git")
  -v, --revision string    the revision of the repo to clone (default "main")
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl pipelines](dctl_pipelines.md)	 - Manage and run draft pipelines

