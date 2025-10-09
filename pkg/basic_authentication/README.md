# Basic Authentication
Is a reusable package that can be dropped into any draft service as a means of quickly authenticating requests on your api's. It's simple username, and password auth but since it's portable and a simple way to get started. It uses the same primitives as the other authentication methods so if you choose to change your strategy hopefully the upgrade is tenable.

## Integrations
Router: Each project might have a different http router that is being used so a simple interface that any system can implement has been defined. Additionally, an implementation using the [chi router](https://github.com/go-chi/chi) has been added in `chi.go` with the interface in `router.go`.

Finally, the storage layer follows the same pattern a reusable interface with a default implementation using [Blueprint](https://github.com/steady-bytes/draft?tab=readme-ov-file#blueprint)

## How to use
1. Copy, and modify the `html/templates` into your service directory at the root in a template folder `./template`

2. Initialize the basic_authentication with the repo, and router configuration. Once initialized add to your service router, and configure your middleware (optionally if authenticating your endpoints) a middleware function has already been included.

```go
// http setup in the service
func NewHTTPHandler(logger chassis.Logger, controller CourseCreatorController, repoUrl string) HTTPHandler {
    // setup the blueprint client, this might be optional depending on your repository layer
	client := kvv1Connect.NewKeyValueServiceClient(&http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
				// If you're also using this client for non-h2c traffic, you may want
				// to delegate to tls.Dial if the network isn't TCP or the addr isn't
				// in an allowlist.
				return net.Dial(network, addr)
			},
		},
	}, repoUrl)

	authRepo := ba.NewBlueprintBasicAuthenticationRepository(logger, client)
	authController := ba.NewBasicAuthenticationController(authRepo)
	authHandler := ba.NewChiBasicAuthenticationHandler(logger, authController)

	return &httpHandler{
		logger:      logger,
		controller:  controller,
		authHandler: authHandler,
	}
}
```

3. Call middleware in your routes
```go
// don't forget to call the middleware when you need to authenticate a route
authHandler.BasicAuthentication()
```