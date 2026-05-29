package models

import (
	"database/sql"
	"time"
)

// Session is a server-side login session.
type Session struct {
	ID        string
	UserID    int64
	CreatedAt string
	ExpiresAt string
	IP        string
	UserAgent string
}

// CreateSession stores a new session with the given id and lifetime.
func CreateSession(db *sql.DB, id string, userID int64, ttl time.Duration, ip, userAgent string) error {
	expires := time.Now().UTC().Add(ttl).Format("2006-01-02 15:04:05")
	_, err := db.Exec(`INSERT INTO sessions (id, user_id, expires_at, ip, user_agent)
		VALUES (?,?,?,?,?)`, id, userID, expires, ip, userAgent)
	return err
}

// GetValidSession returns the session for id only if it has not expired.
func GetValidSession(db *sql.DB, id string) (Session, error) {
	row := db.QueryRow(`SELECT id, user_id, created_at, expires_at, ip, user_agent
		FROM sessions WHERE id = ? AND expires_at > datetime('now')`, id)
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.IP, &s.UserAgent)
	return s, err
}

// DeleteSession removes a single session (logout).
func DeleteSession(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteUserSessions removes all sessions for a user (logout everywhere /
// used when deactivating an account).
func DeleteUserSessions(db *sql.DB, userID int64) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteExpiredSessions purges sessions past their expiry. Called periodically.
func DeleteExpiredSessions(db *sql.DB) (int64, error) {
	res, err := db.Exec(`DELETE FROM sessions WHERE expires_at <= datetime('now')`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
