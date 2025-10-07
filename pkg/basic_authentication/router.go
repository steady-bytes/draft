package basic_authentication

import (
	"net/http"
)

type BasicAuthenticationHandler interface {
	RenderRegistrationPage(w http.ResponseWriter, r *http.Request)
	HandleRegistrationPost(w http.ResponseWriter, r *http.Request)
	RenderLoginPage(w http.ResponseWriter, r *http.Request)
	HandleLoginPost(w http.ResponseWriter, r *http.Request)
	HandleLogoutPost(w http.ResponseWriter, r *http.Request)
	RefreshAuthToken(w http.ResponseWriter, r *http.Request)

	// middlewares for enforcing authentication
	BasicAuthenticationMiddleware(next http.Handler) http.Handler
}
