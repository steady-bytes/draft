# Authentication
This is a loaded topic and most likely wont be implemented by hand for a development team. In most companies each developer is given the tools to use
auth when working on the their project. Since draft is aiming to be platform framework its should have some type of auth. Now as far as having implemented services with customer interfaces that may not be draft b/c I normally like to just pickup something off the shelf and then integrate my system with that Authentik [] and Authealia [] come to mind.

For my imediate case though I'll be implementing basic authentication. It's  probably a good idea to document how the following can be implemented or used with Draft[].

## OAuth

## API Keys

## Basic Authentication
Basic will first exist as a set of globally defined types like `user`, and `session`. an a generic package which will contain an interface, and a default implementation using blueprint as it's data store. In the future integrations with postgres or other datastores would really be helpful.

## JWT Authentication

## OpenID Connect Authentication

### Research on Auth
Below are the most common forms of authentication implemented in most web applications and software systems. Draft must support a path forward for each of the below to be a viable option for enterprise grade systems.

Username and Password (Basic Authentication):
Users log in with a username/email and password. Often combined with session cookies.

Token-Based Authentication (e.g., JWT):
Users receive a signed token (like a JWT) after login, which is sent with each request for stateless authentication.

OAuth 2.0:
Delegated authentication using third-party providers (Google, Facebook, GitHub, etc.). Common for "Login with X" buttons.

API Keys:
Used for authenticating API requests, especially for service-to-service communication.

Multi-Factor Authentication (MFA/2FA):
Requires a second factor (e.g., SMS code, authenticator app) in addition to password.

OpenID Connect:
An identity layer on top of OAuth 2.0, often used for single sign-on (SSO).

SAML:
Used for enterprise SSO, especially in corporate environments.