package api

import (
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"aarvix-nexus/internal/auth"
)

// Used to burn the same CPU time for unknown emails (blocks user enumeration by timing).
var dummyHash, _ = auth.HashPassword("dummy-password-123")

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

func normEmail(s string) (string, bool) {
	e := strings.ToLower(strings.TrimSpace(s))
	if len(e) > 254 {
		return "", false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Address != e {
		return "", false
	}
	return e, true
}

func (a *API) sendCode(email, code string) {
	body := "Your AARVIX NEXUS verification code is " + code + ". It expires in 15 minutes."
	if err := a.mail.Send(email, "AARVIX NEXUS verification code", body); err != nil {
		// logged, but the API response stays identical so nothing is leaked
		log.Printf("mail error: %v", err)
	}
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	email, ok := normEmail(in.Email)
	if !ok {
		errJSON(w, 400, "invalid email")
		return
	}
	if err := auth.ValidPassword(in.Password); err != nil {
		errJSON(w, 400, err.Error())
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	code := auth.NewCode()
	tag, err := a.pool.Exec(r.Context(), `
		INSERT INTO users(id, email, password_hash, verify_hash, verify_expires)
		VALUES ($1,$2,$3,$4, now() + interval '15 minutes')
		ON CONFLICT (email) DO UPDATE
		   SET password_hash=EXCLUDED.password_hash, verify_hash=EXCLUDED.verify_hash,
		       verify_expires=EXCLUDED.verify_expires, failed_logins=0
		   WHERE users.verified = false`,
		uuid.NewString(), email, hash, auth.HashCode(code))
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	if tag.RowsAffected() > 0 {
		a.sendCode(email, code)
	}
	// Same answer whether or not the email already exists (no account enumeration).
	writeJSON(w, 202, map[string]string{"message": "if the email is valid, a verification code was sent"})
}

func (a *API) resend(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	if email, ok := normEmail(in.Email); ok {
		code := auth.NewCode()
		tag, err := a.pool.Exec(r.Context(), `
			UPDATE users SET verify_hash=$2, verify_expires=now()+interval '15 minutes', failed_logins=0
			WHERE email=$1 AND verified=false`, email, auth.HashCode(code))
		if err == nil && tag.RowsAffected() > 0 {
			a.sendCode(email, code)
		}
	}
	writeJSON(w, 202, map[string]string{"message": "if the account needs verification, a new code was sent"})
}

func (a *API) verify(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	email, ok := normEmail(in.Email)
	if !ok || len(in.Code) != 6 {
		errJSON(w, 400, "invalid or expired code")
		return
	}
	var id string
	var hash *string
	var exp *time.Time
	err := a.pool.QueryRow(r.Context(),
		`SELECT id, verify_hash, verify_expires FROM users WHERE email=$1 AND verified=false`, email).Scan(&id, &hash, &exp)
	if err != nil || hash == nil || exp == nil || exp.Before(time.Now()) {
		errJSON(w, 400, "invalid or expired code")
		return
	}
	if !auth.CodeMatches(*hash, in.Code) {
		// 5 wrong guesses invalidate the code (stops brute-forcing the 6 digits)
		_, _ = a.pool.Exec(r.Context(), `
			UPDATE users SET failed_logins=failed_logins+1,
			       verify_hash = CASE WHEN failed_logins+1 >= 5 THEN NULL ELSE verify_hash END
			WHERE id=$1`, id)
		errJSON(w, 400, "invalid or expired code")
		return
	}
	_, err = a.pool.Exec(r.Context(),
		`UPDATE users SET verified=true, verify_hash=NULL, verify_expires=NULL, failed_logins=0 WHERE id=$1`, id)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	a.audit(r.Context(), email, "email_verified", "")
	writeJSON(w, 200, map[string]string{"message": "email verified, you can log in"})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var in credentials
	if !decode(w, r, &in) {
		return
	}
	email, ok := normEmail(in.Email)
	if !ok {
		errJSON(w, 401, "invalid credentials")
		return
	}
	var id, hash, role string
	var verified bool
	var failed int
	var locked *time.Time
	err := a.pool.QueryRow(r.Context(),
		`SELECT id, password_hash, role, verified, failed_logins, locked_until FROM users WHERE email=$1`,
		email).Scan(&id, &hash, &role, &verified, &failed, &locked)
	if err != nil {
		auth.CheckPassword(dummyHash, in.Password)
		errJSON(w, 401, "invalid credentials")
		return
	}
	if locked != nil && locked.After(time.Now()) {
		errJSON(w, 429, "account temporarily locked, try again later")
		return
	}
	if !auth.CheckPassword(hash, in.Password) {
		if failed+1 >= 5 { // lock for 15 minutes after 5 consecutive failures
			_, _ = a.pool.Exec(r.Context(),
				`UPDATE users SET failed_logins=0, locked_until=now()+interval '15 minutes' WHERE id=$1`, id)
			a.audit(r.Context(), email, "account_locked", clientIP(r))
		} else {
			_, _ = a.pool.Exec(r.Context(), `UPDATE users SET failed_logins=failed_logins+1 WHERE id=$1`, id)
		}
		errJSON(w, 401, "invalid credentials")
		return
	}
	if !verified {
		errJSON(w, 403, "email_not_verified")
		return
	}
	_, _ = a.pool.Exec(r.Context(), `UPDATE users SET failed_logins=0, locked_until=NULL WHERE id=$1`, id)
	tok, err := auth.Sign(a.secret, id, email, role, 30*time.Minute)
	if err != nil {
		errJSON(w, 500, "internal error")
		return
	}
	a.audit(r.Context(), email, "login", clientIP(r))
	writeJSON(w, 200, map[string]string{"token": tok, "role": role, "email": email})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	c := claimsOf(r)
	writeJSON(w, 200, map[string]string{"id": c.Subject, "email": c.Email, "role": c.Role})
}
