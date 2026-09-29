package draftui

// AuthField is one input of an AuthCard.
type AuthField struct {
	Label, Name, Type string
	Placeholder       string
	// Autocomplete is the HTML autocomplete token ("username", "current-password", "new-password").
	Autocomplete string
	Required     bool
}

// AuthCard is the model of the `auth-card` partial: a centred card with a title, an optional error,
// its fields, a submit button and a link to the sibling page.
type AuthCard struct {
	Action string
	// Eyebrow is the small caps line over the title; Title is the heading. Both are neutral by
	// default: the card belongs to whichever service embeds it.
	Eyebrow, Title string
	Error          string
	Fields         []AuthField
	// Remember adds a "Remember me" checkbox (name remember-me).
	Remember bool
	Submit   string
	// AltText, AltLabel and AltHref are the "No account? Register" line.
	AltText, AltLabel, AltHref string
}

// LoginCard is the sign-in card posting to action (username, password, remember-me).
func LoginCard(action string) AuthCard {
	return AuthCard{
		Action:  action,
		Eyebrow: "Account",
		Title:   "Sign in",
		Fields: []AuthField{
			{Label: "Username", Name: "username", Type: "text", Placeholder: "you@example.com", Autocomplete: "username", Required: true},
			{Label: "Password", Name: "password", Type: "password", Autocomplete: "current-password", Required: true},
		},
		Remember: true,
		Submit:   "Sign in",
		AltText:  "No account yet?",
		AltLabel: "Register",
		AltHref:  "/register",
	}
}

// RegisterCard is the create-account card posting to action. userExists shows the "already exists"
// error the basic_authentication handler redirects with.
func RegisterCard(action string, userExists bool) AuthCard {
	card := AuthCard{
		Action:  action,
		Eyebrow: "Account",
		Title:   "Create an account",
		Fields: []AuthField{
			{Label: "Username", Name: "username", Type: "text", Placeholder: "you@example.com", Autocomplete: "username", Required: true},
			{Label: "Password", Name: "password", Type: "password", Autocomplete: "new-password", Required: true},
			{Label: "Confirm password", Name: "password-confirmation", Type: "password", Autocomplete: "new-password", Required: true},
		},
		Submit:   "Create account",
		AltText:  "Already registered?",
		AltLabel: "Sign in",
		AltHref:  "/login",
	}
	if userExists {
		card.Error = "That username is taken. Choose a different one."
	}
	return card
}
