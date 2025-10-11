package basic_authentication

import (
	"errors"
	"net/http"
	"text/template"
	"time"

	"github.com/go-chi/chi"
	"github.com/steady-bytes/draft/pkg/chassis"

	bav1 "github.com/steady-bytes/draft/api/core/authentication/basic/v1"
)

// RegisterDefaultAuthRoutes registers login/logout routes on the given router.
func RegisterDefaultAuthRoutes(r *chi.Mux, h BasicAuthenticationHandler) chi.Router {
	// Public server side pages for registration and login
	r.Get("/register", h.RenderRegistrationPage) // Register the registration handler
	r.Get("/login", h.RenderLoginPage)           // Register the login handler

	// Form submission handlers
	r.Post("/register", h.HandleRegistrationPost) // Handle registration form submission
	r.Post("/login", h.HandleLoginPost)           // Handle login form submission

	// protected routes for logout and token refresh
	r.Group(func(protected chi.Router) {
		protected.Use(h.BasicAuthenticationMiddleware)       // Middleware to protect routes that require authentication
		protected.Post("/logout", h.HandleLogoutPost)        // Handle logout requests
		protected.Post("/refresh-token", h.RefreshAuthToken) // Handle token refresh requests
	})

	return r
}

func NewChiBasicAuthenticationHandler(logger chassis.Logger, controller BasicAuthentication) BasicAuthenticationHandler {
	return &chiBasicAuthenticationHandler{
		logger:     logger,
		controller: controller,
	}
}

type chiBasicAuthenticationHandler struct {
	logger     chassis.Logger
	controller BasicAuthentication
}

type registrationPageData struct {
	IsAlreadyRegistered bool
}

func (h *chiBasicAuthenticationHandler) RenderRegistrationPage(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("registration page with chi router")

	pd := registrationPageData{}
	if r.URL.Query().Get("error") == "user_exists" {
		pd.IsAlreadyRegistered = true
	}

	tmpl, err := template.ParseFiles(
		"services/golf-app/app/templates/index.html",
		"services/golf-app/app/templates/register.html",
	)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		h.logger.Debug("failed to parse registration template: " + err.Error())
		return
	}

	// You can pass data to the template if needed, here we pass nil
	if err := tmpl.Execute(w, pd); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		h.logger.Debug("failed to execute registration template: " + err.Error())
		return
	}
}

func (h *chiBasicAuthenticationHandler) HandleRegistrationPost(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("handling registration request")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		h.logger.Debug("failed to parse form: " + err.Error())
		return
	}

	h.logger.Debug("parsed form successfully")

	username := r.FormValue("username")
	if username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		h.logger.Debug("username is required")
		return
	}

	password := r.FormValue("password")
	if password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		h.logger.Debug("password is required")
		return
	}

	passwordConfirm := r.FormValue("password-confirmation")
	if passwordConfirm == "" {
		http.Error(w, "Password confirmation is required", http.StatusBadRequest)
		h.logger.Debug("password confirmation is required")
		return
	}

	if password != passwordConfirm {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		h.logger.Debug("passwords do not match")
		return
	}

	h.logger.Debug("registration form is valid, proceeding")

	_, err := h.controller.Register(r.Context(), &bav1.Entity{
		Username: username,
		Password: password,
	})
	if err != nil {
		h.logger.Debug("failed to register user: " + err.Error())

		// TODO: handle specific error cases, e.g., user already exists
		if errors.Is(err, ErrUserAlreadyExists) {
			h.logger.Debug("user already exists: " + username)
			http.Redirect(w, r, "/register?error=user_exists", http.StatusSeeOther)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *chiBasicAuthenticationHandler) RenderLoginPage(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("login page with chi router")

	// now I need to implement the login page handler
	// I think I'm going to user some type of http template to render the login page.

	tmpl, err := template.ParseFiles(
		"services/golf-app/app/templates/index.html",
		"services/golf-app/app/templates/login.html",
	)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		h.logger.Debug("failed to parse login template: " + err.Error())
		return
	}

	// You can pass data to the template if needed, here we pass nil
	if err := tmpl.Execute(w, nil); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		h.logger.Debug("failed to execute login template: " + err.Error())
		return
	}
}

func (h *chiBasicAuthenticationHandler) HandleLoginPost(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("handling login request")

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		h.logger.Debug("failed to parse form: " + err.Error())
		return
	}

	username := r.FormValue("username")
	if username == "" {
		http.Error(w, "Username is required", http.StatusBadRequest)
		h.logger.Debug("username is required")
		return
	}

	password := r.FormValue("password")
	if password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		h.logger.Debug("password is required")
		return
	}

	rememberMe := r.FormValue("remember-me") == "on"

	session, err := h.controller.Login(r.Context(), &bav1.Entity{
		Username: username,
		Password: password,
	}, rememberMe)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.Debug("failed to login user: " + err.Error())
		return
	}

	// redirect the user to the home page of the application after successful with a private cookie
	// that contains the session token that can be used to authenticate the user in subsequent requests.

	// Set a secure, HTTP-only cookie with the session token
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    session.Token, // assuming session.Token is your JWT or session string
		Path:     "/",
		HttpOnly: true, // not accessible via JS
		Secure:   true, // only sent over HTTPS (set to false for local dev if not using HTTPS)
		SameSite: http.SameSiteStrictMode,
		Expires:  session.ExpiresAt.AsTime(),
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    session.RefreshToken, // assuming session.Token is your JWT or session string
		Path:     "/",
		HttpOnly: true, // not accessible via JS
		Secure:   true, // only sent over HTTPS (set to false for local dev if not using HTTPS)
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour), // or time.Now().Add(24 * time.Hour)
	})

	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *chiBasicAuthenticationHandler) RefreshAuthToken(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("handling token refresh request")

	// Get the refresh token from the cookie
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.Debug("no refresh token cookie found")
		return
	}

	if err := h.controller.ValidateToken(r.Context(), cookie.Value); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.Debug("failed to validate refresh token: " + err.Error())
		return
	}

	// Generate a new access token
	newAccessToken, err := h.controller.RefreshAuthToken(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		h.logger.Debug("failed to generate new access token: " + err.Error())
		return
	}

	// Set the new access token in the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    newAccessToken.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Minute),
	})

	w.WriteHeader(http.StatusOK)
}

func (h *chiBasicAuthenticationHandler) HandleLogoutPost(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("handling logout request")

	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.Debug("no refresh token cookie found")
		return
	}

	if err := h.controller.ValidateToken(r.Context(), cookie.Value); err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		h.logger.Debug("failed to validate refresh token: " + err.Error())
		return
	}

	h.controller.Logout(r.Context(), cookie.Value)

	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour),
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ///////////
// MIDDLEWARE
// ///////////

// BasicAuthenticationMiddleware is a middleware that checks if the user is authenticated using the basic authentication scheme.
func (h *chiBasicAuthenticationHandler) BasicAuthenticationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the user is authenticated
		cookie, err := r.Cookie("auth_token")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// check the session token and it's validity
		if err := h.controller.ValidateToken(r.Context(), cookie.Value); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			h.logger.Debug("failed to parse JWT token: " + err.Error())
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// If authenticated, proceed to the next handler
		next.ServeHTTP(w, r)
	})
}
