## dctl release image

Release a Docker image

### Synopsis

Release a Docker image by adding a tag to an existing image (e.g. latest, edge). This will pull the existing image,
apply the new tag, and then push the image. You must set the env variables CONTAINER_REGISTRY_USERNAME and CONTAINER_REGISTRY_PASSWORD
in order to authenticate to the registry.

```
dctl release image [flags]
```

### Examples

```
dctl release image --image registry.com/hello/world --source latest --target v1.0.0
```

### Options

```
  -h, --help            help for image
  -i, --image string    the Docker image to release/tag
  -s, --source string   the source image tag to pull
  -t, --target string   the target image tag to apply and push
```

### Options inherited from parent commands

```
      --config string    config file
      --context string   override the current context
```

### SEE ALSO

* [dctl release](dctl_release.md)	 - Commands for releasing Draft components

