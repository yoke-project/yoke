package interfaces

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
	"time"
)

// SessionCookie is the cookie a browser carries its session token in: one the page cannot read.
const SessionCookie = "yoke-session"

// SessionLife is how long a session token stands from its issue, and never past the Core's life.
const SessionLife = 12 * time.Hour

// sessions are the tokens one channel issued, held in memory and nowhere else: a token stands for one
// account on this channel alone, an account holds one at a time, and unbinding the channel lets them all
// go with it.
type sessions struct {
	mu        sync.Mutex
	byKey     map[[32]byte]*held
	byAccount map[string][32]byte
}

type held struct {
	account string
	token   []byte
	expires time.Time
}

// issue makes a token for account, ending the one it held: 32 bytes from the host's random source, as
// base64url.
func (s *sessions) issue(account string, now time.Time) (string, time.Time) {
	raw := make([]byte, 32)
	rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	key := sha256.Sum256([]byte(token))
	expires := now.Add(SessionLife)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byKey == nil {
		s.byKey, s.byAccount = map[[32]byte]*held{}, map[string][32]byte{}
	}
	if old, ok := s.byAccount[account]; ok {
		delete(s.byKey, old)
	}
	s.byKey[key] = &held{account: account, token: []byte(token), expires: expires}
	s.byAccount[account] = key
	return token, expires
}

// redeem is the account a token stands for, while it stands. The token is found by its digest and then
// compared in constant time, as every credential is.
func (s *sessions) redeem(token string, now time.Time) (string, time.Time, bool) {
	key := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	h, found := s.byKey[key]
	if !found || subtle.ConstantTimeCompare(h.token, []byte(token)) != 1 {
		return "", time.Time{}, false
	}
	if !now.Before(h.expires) {
		delete(s.byKey, key)
		delete(s.byAccount, h.account)
		return "", time.Time{}, false
	}
	return h.account, h.expires, true
}
