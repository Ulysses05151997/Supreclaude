package models

import (
	"database/sql"
)

// User is a staff account.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string // 'admin' | 'staff'
	DisplayName  string
	IsActive     bool
	MustChangePW bool
	CreatedAt    string
	UpdatedAt    string
}

// IsAdmin reports whether the user has the admin role.
func (u User) IsAdmin() bool { return u.Role == "admin" }

// CountUsers returns the total number of user rows (used to detect first-run).
func CountUsers(db *sql.DB) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// GetUserByUsername looks up a user by case-insensitive username.
func GetUserByUsername(db *sql.DB, username string) (User, error) {
	row := db.QueryRow(`SELECT id, username, password_hash, role, display_name,
		is_active, must_change_pw, created_at, updated_at
		FROM users WHERE username = ? COLLATE NOCASE`, username)
	return scanUser(row)
}

// GetUser looks up a user by id.
func GetUser(db *sql.DB, id int64) (User, error) {
	row := db.QueryRow(`SELECT id, username, password_hash, role, display_name,
		is_active, must_change_pw, created_at, updated_at
		FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// ListUsers returns all users ordered by username.
func ListUsers(db *sql.DB) ([]User, error) {
	rows, err := db.Query(`SELECT id, username, password_hash, role, display_name,
		is_active, must_change_pw, created_at, updated_at
		FROM users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CreateUser inserts a new user and returns its id.
func CreateUser(db *sql.DB, username, passwordHash, role, displayName string, mustChangePW bool) (int64, error) {
	res, err := db.Exec(`INSERT INTO users
		(username, password_hash, role, display_name, must_change_pw)
		VALUES (?,?,?,?,?)`,
		username, passwordHash, role, displayName, boolToInt(mustChangePW))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetPassword updates a user's password hash and clears the must-change flag.
func SetPassword(db *sql.DB, userID int64, passwordHash string) error {
	_, err := db.Exec(`UPDATE users SET password_hash=?, must_change_pw=0,
		updated_at=datetime('now') WHERE id=?`, passwordHash, userID)
	return err
}

// SetActive activates or deactivates a user.
func SetActive(db *sql.DB, userID int64, active bool) error {
	_, err := db.Exec(`UPDATE users SET is_active=?, updated_at=datetime('now')
		WHERE id=?`, boolToInt(active), userID)
	return err
}

func scanUser(s scanner) (User, error) {
	var u User
	var active, mustChange int
	err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName,
		&active, &mustChange, &u.CreatedAt, &u.UpdatedAt)
	u.IsActive = active != 0
	u.MustChangePW = mustChange != 0
	return u, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
