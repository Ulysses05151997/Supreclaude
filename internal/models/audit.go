package models

import (
	"database/sql"
)

// AuditEvent records a notable action for the firm's records.
type AuditEvent struct {
	ID         int64
	UserID     sql.NullInt64
	Action     string
	EntityType string
	EntityID   string
	Detail     string
	IP         string
	CreatedAt  string
	// Username is populated by ListAuditEvents for display (join), not stored.
	Username string
}

// LogAudit writes an audit event. Failures are non-fatal to the caller's flow;
// callers should log the error but not abort the user's action.
func LogAudit(db *sql.DB, userID int64, action, entityType, entityID, detail, ip string) error {
	var uid any
	if userID > 0 {
		uid = userID
	}
	_, err := db.Exec(`INSERT INTO audit_events
		(user_id, action, entity_type, entity_id, detail, ip)
		VALUES (?,?,?,?,?,?)`, uid, action, entityType, entityID, detail, ip)
	return err
}

// ListAuditEvents returns the most recent events (up to limit), newest first,
// joined to the username for display.
func ListAuditEvents(db *sql.DB, limit int) ([]AuditEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.Query(`SELECT a.id, a.user_id, a.action, a.entity_type,
		a.entity_id, a.detail, a.ip, a.created_at, COALESCE(u.username, '')
		FROM audit_events a LEFT JOIN users u ON u.id = a.user_id
		ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.EntityType,
			&e.EntityID, &e.Detail, &e.IP, &e.CreatedAt, &e.Username); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
