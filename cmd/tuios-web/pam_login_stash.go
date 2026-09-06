package main

import (
	"sync"
	"time"

	"github.com/tonk/tuios/internal/pamauth"
)

// pendingPAMLoginTTL is how long a Login dialed on the page-load "/" request
// is held for the WebSocket upgrade to claim. Long enough to cover font/
// script fetch + the (now skipped) WebTransport attempt, short enough that a
// tab that never upgrades does not pin a pam-helper session forever.
const pendingPAMLoginTTL = 30 * time.Second

// pendingPAMLogin is one Login produced by the front door on "/" and waiting
// for pamAuthMiddleware to claim it on the WebSocket upgrade, so the expensive
// pam_unix password hash (SHA-512 with hundreds of thousands of rounds on
// some lab hosts) runs once per page load instead of twice.
type pendingPAMLogin struct {
	login   *pamauth.Login
	credKey [32]byte
	expires time.Time
}

var pendingPAMLogins sync.Map // sid string -> *pendingPAMLogin

// stashPendingPAMLogin stores login under sid for takePendingPAMLogin.
// Any previous pending Login for the same sid is closed.
func stashPendingPAMLogin(sid, username, password string, login *pamauth.Login) {
	if sid == "" || login == nil {
		return
	}
	entry := &pendingPAMLogin{
		login:   login,
		credKey: verifiedCredentialKey(username, password),
		expires: time.Now().Add(pendingPAMLoginTTL),
	}
	if prev, loaded := pendingPAMLogins.Swap(sid, entry); loaded {
		if old, ok := prev.(*pendingPAMLogin); ok && old != nil && old.login != nil {
			_ = old.login.Close()
		}
	}
	time.AfterFunc(pendingPAMLoginTTL, func() {
		expirePendingPAMLogin(sid, entry)
	})
}

func expirePendingPAMLogin(sid string, entry *pendingPAMLogin) {
	cur, ok := pendingPAMLogins.Load(sid)
	if !ok || cur != entry {
		return
	}
	pendingPAMLogins.Delete(sid)
	if entry.login != nil {
		_ = entry.login.Close()
	}
}

// takePendingPAMLogin claims a stashed Login for this sid when the Basic Auth
// credentials match what was dialed. The caller owns the returned Login.
// A miss (wrong password, expired, already claimed, no cookie) returns nil
// and the caller must Dial itself.
func takePendingPAMLogin(sid, username, password string) *pamauth.Login {
	if sid == "" {
		return nil
	}
	cur, ok := pendingPAMLogins.Load(sid)
	if !ok {
		return nil
	}
	entry, ok := cur.(*pendingPAMLogin)
	if !ok || entry == nil {
		return nil
	}
	if time.Now().After(entry.expires) {
		pendingPAMLogins.Delete(sid)
		if entry.login != nil {
			_ = entry.login.Close()
		}
		return nil
	}
	if entry.credKey != verifiedCredentialKey(username, password) {
		return nil
	}
	if !pendingPAMLogins.CompareAndDelete(sid, entry) {
		return nil
	}
	return entry.login
}
