package service

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/supertokens/supertokens-golang/recipe/session"
	"github.com/supertokens/supertokens-golang/recipe/session/sessmodels"
)

// This is a function that wraps the supertokens verification function
// to work the gin
func verifySession(options *sessmodels.VerifySessionOptions) gin.HandlerFunc {
	return func(c *gin.Context) {
		session.VerifySession(options, func(rw http.ResponseWriter, r *http.Request) {
			c.Request = c.Request.WithContext(r.Context())
			c.Next()
		})(c.Writer, c.Request)
		// we call Abort so that the next handler in the chain is not called, unless we call Next explicitly
		c.Abort()
	}
}

func likeCommentAPI(c *gin.Context) {
	// retrieve the session object as shown below
	sessionContainer := session.GetSessionFromRequestContext(c.Request.Context())

	userID := sessionContainer.GetUserID()

	log.
		Debug().
		Str("userID", userID).
		Msg("protected route")

	c.JSON(http.StatusOK, gin.H{
		"message": "protected route",
	})
}
