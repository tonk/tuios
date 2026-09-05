package main

import (
	"context"
	"log"
	"net/http"

	"github.com/Gaurav-Gosain/sip"
	"github.com/tonk/tuios/internal/config"
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
// session (or, under the [classroom] trainer console, to
// createClassroomTUIOSInstance instead - see main.go).
//
// classroom is the loaded [classroom] config, used only to authorize a
// trainer's own request (via the "attach" query parameter, see
// classroomAttachTargetFromContext) to view another trainee's session - the
// zero value (trainer console off) makes that check always fail closed.
//
// This only ever runs when --pam-auth was passed; see main.go.
func pamAuthMiddleware(socketPath string, classroom config.ClassroomConfig) sip.ConnectMiddleware {
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
			ctx := sip.WithIdentity(r.Context(), login)

			// A trainer requesting another trainee's session by name. Checked
			// here, server-side, against the account that PAM just verified -
			// never trusted to anything the client claims about itself. A
			// denied request gets the same generic "authentication failed" as
			// a wrong password: which check it tripped is not something to
			// hand an attacker either.
			if target := r.URL.Query().Get("attach"); target != "" && target != username {
				if !classroom.IsTrainer(username) || !classroom.MatchesTrainee(target) {
					log.Printf("classroom attach denied: %q requested session %q", username, target)
					_ = login.Close()
					return unauthorized("Authentication failed")
				}
				ctx = withClassroomAttachTarget(ctx, target)
			}

			return next(r.WithContext(ctx))
		}
	}
}

// classroomAttachTargetKey is an unexported context-key type (the standard
// context idiom to avoid collisions) for the trainer-requested session name
// pamAuthMiddleware resolves. It is a plain context.WithValue, not the
// sip.WithIdentity mechanism the Login itself rides on: that mechanism
// carries exactly one identity value, already spoken for.
type classroomAttachTargetKey struct{}

func withClassroomAttachTarget(ctx context.Context, target string) context.Context {
	return context.WithValue(ctx, classroomAttachTargetKey{}, target)
}

// classroomAttachTargetFromContext reads back the session name an authorized
// trainer's "attach" query parameter resolved to, if any.
func classroomAttachTargetFromContext(ctx context.Context) (string, bool) {
	target, ok := ctx.Value(classroomAttachTargetKey{}).(string)
	return target, ok
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
