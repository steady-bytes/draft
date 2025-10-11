package basic_authentication

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	bav1 "github.com/steady-bytes/draft/api/core/authentication/basic/v1"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BasicAuthentication is the service interface for a basic authentication system.
// It defines the methods required for logging in and logging out users.
type BasicAuthentication interface {
	Register(ctx context.Context, entity *bav1.Entity) (*bav1.Entity, error)
	Login(ctx context.Context, entity *bav1.Entity, remember bool) (*bav1.Session, error)
	Logout(ctx context.Context, refreshToken string) error
	RefreshAuthToken(ctx context.Context, refreshToken string) (*bav1.Session, error)
	ValidateToken(ctx context.Context, token string) error
}

var (
	ErrUserAlreadyExists = errors.New("user already exists")
)

func NewBasicAuthenticationController(repo BasicAuthenticationRepository) BasicAuthentication {
	return &basicAuthenticationController{
		repository: repo,
	}
}

type basicAuthenticationController struct {
	repository BasicAuthenticationRepository
}

func (c *basicAuthenticationController) Register(ctx context.Context, entity *bav1.Entity) (*bav1.Entity, error) {
	found, err := c.repository.Get(ctx, bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_USERNAME, entity.Username)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			return nil, fmt.Errorf("failed to check if user exists: %w", err)
		}
	}

	if found != nil {
		return nil, ErrUserAlreadyExists
	}

	entity.Password, err = c.hashPassword(entity.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	savedEntity, err := c.repository.SaveEntity(ctx, entity)
	if err != nil {
		return nil, fmt.Errorf("failed to save user: %w", err)
	}

	return savedEntity, nil
}

func (c *basicAuthenticationController) Login(ctx context.Context, entity *bav1.Entity, remember bool) (*bav1.Session, error) {
	storedEntity, err := c.repository.Get(ctx, bav1.LookupEntityKeys_LOOKUP_ENTITY_KEY_USERNAME, entity.Username)
	if err != nil {
		return nil, err
	}

	if err := c.checkPassword(storedEntity.Password, entity.Password); err != nil {
		return nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	accessToken, err := c.generateAccessToken(storedEntity.Username)
	if err != nil {
		return nil, err
	}

	session := &bav1.Session{
		Id:        uuid.NewString(),
		UserId:    entity.Id,
		CreatedAt: timestamppb.Now(),
		Token:     accessToken,
		// todo make this configurable
		ExpiresAt: timestamppb.New(time.Now().Add(24 * time.Hour)), // Default to 24 hours
	}

	if remember {
		// if remember me is true, create a long-lived session
		refreshToken, err := c.generateRefreshToken(storedEntity.Username, session.Id)
		if err != nil {
			return nil, err
		}
		session.RefreshToken = refreshToken

		if _, err := c.repository.SaveSession(ctx, session, storedEntity.Username); err != nil {
			return nil, err
		}
	}

	return session, nil
}

func (c *basicAuthenticationController) Logout(ctx context.Context, refreshToken string) error {
	// Parse the JWT token
	jwtToken, err := c.parseJWT(refreshToken)
	if err != nil {
		return err
	}

	if claims, ok := jwtToken.Claims.(jwt.MapClaims); ok && jwtToken.Valid {
		username := claims["username"].(string)
		sessionID := claims["session"].(string)
		return c.repository.DeleteSession(ctx, &bav1.Session{Id: sessionID}, username)
	}

	return errors.New("invalid refresh token")
}

func (c *basicAuthenticationController) ValidateToken(ctx context.Context, token string) error {
	if _, err := c.parseJWT(token); err != nil {
		return err
	}

	return nil
}

func (c *basicAuthenticationController) RefreshAuthToken(ctx context.Context, refreshToken string) (*bav1.Session, error) {
	// Parse the JWT token
	jwtToken, err := c.parseJWT(refreshToken)
	if err != nil {
		return nil, err
	}

	// Validate the token and extract claims
	if claims, ok := jwtToken.Claims.(jwt.MapClaims); ok && jwtToken.Valid {
		username := claims["username"].(string)
		sessionID := claims["session"].(string)
		newAccessToken, err := c.generateAccessToken(username)
		if err != nil {
			return nil, err
		}
		return &bav1.Session{
			Id:           sessionID,
			UserId:       username,
			Token:        newAccessToken,
			RefreshToken: refreshToken,
			CreatedAt:    timestamppb.Now(),
			ExpiresAt:    timestamppb.New(time.Now().Add(24 * time.Minute)), // Default to 24 minutes
		}, nil
	}

	return nil, errors.New("invalid refresh token")
}

////////////
// UTILITIES
////////////

// HashPassword hashes a plain-text password using bcrypt.
func (c *basicAuthenticationController) hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// CheckPassword compares a bcrypt hashed password with its possible plaintext equivalent.
func (c *basicAuthenticationController) checkPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// TODO: Read in secret key from environment variable or configuration file
// Replace with your own secret key (keep it safe!)
var jwtSecret = []byte("your-very-secret-key")

func (c *basicAuthenticationController) generateAccessToken(username string) (string, error) {
	// Set custom and standard claims
	claims := jwt.MapClaims{
		"username": username,
		"exp":      time.Now().Add(24 * time.Minute).Unix(), // Expires in 24 minutes
		"iat":      time.Now().Unix(),                       // Issued at
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with your secret
	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func (c *basicAuthenticationController) generateRefreshToken(username, sessionID string) (string, error) {
	// Set custom and standard claims
	claims := jwt.MapClaims{
		"username": username,
		"session":  sessionID,
		"exp":      time.Now().Add(24 * time.Hour).Unix(), // Expires in 24 hours
		"iat":      time.Now().Unix(),                     // Issued at
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with your secret
	signedToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}
	return signedToken, nil
}

func (c *basicAuthenticationController) parseJWT(tokenString string) (*jwt.Token, error) {
	return jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return jwtSecret, nil
	})
}
