package main

import (
	"context"
	"log"
	"net/http"

	"github.com/Gaurav-Gosain/sip"
	"github.com/tonk/tuios/internal/pamauth"
)

// pamAuthMiddleware gates every connection behind an HTTP Basic Auth prompt,
// verified against the (separately-run, privileged) pam-helper process at
// socketPath. On success the resulting *pamauth.Login - a live, authenticated
// connection able to spawn as many shells as this trainee opens windows for -
// is carried into the session context via sip.WithIdentity, exactly as
// touchMiddleware carries its own answer (see touch.go). createTUIOSHandler
// reads it back out and, when present, hands it to app.NewOS as
// OSOptions.PAMLogin instead of building an ordinary local or daemon-backed
// session.
//
// This only ever runs when --pam-auth was passed; see main.go.
func pamAuthMiddleware(socketPath string) sip.ConnectMiddleware {
	return func(next sip.ConnectHandler) sip.ConnectHandler {
		return func(r *http.Request) error {
			username, password, ok := r.BasicAuth()
			if !ok {
				return unauthorized("Authentication required")
			}
			login, err := pamauth.Dial(socketPath, username, password)
			if err != nil {
				// The specific reason (wrong password vs. helper unreachable
				// vs. account locked) is only useful server-side: handing it
				// to the browser would tell an attacker which they hit.
				log.Printf("PAM login for %q failed: %v", username, err)
				return unauthorized("Authentication failed")
			}
			return next(r.WithContext(sip.WithIdentity(r.Context(), login)))
		}
	}
}

func unauthorized(body string) error {
	return &sip.ConnectError{
		Status:  http.StatusUnauthorized,
		Headers: http.Header{"WWW-Authenticate": {`Basic realm="tuios-web"`}},
		Body:    body,
	}
}

// pamLoginFromContext reads back what pamAuthMiddleware decided, if
// --pam-auth authenticated this connection.
func pamLoginFromContext(ctx context.Context) (*pamauth.Login, bool) {
	id, ok := sip.IdentityFromContext(ctx)
	if !ok {
		return nil, false
	}
	login, ok := id.(*pamauth.Login)
	return login, ok
}
