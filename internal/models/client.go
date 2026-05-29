package models

import (
	"database/sql"
	"strings"
)

// Client is a law-firm client record. folder_path is relative to the
// configured case-files root.
type Client struct {
	ID         int64
	Name       string
	Address    string
	Phone      string
	Email      string
	Birthday   string // 'YYYY-MM-DD'
	CaseType   string
	Adversary  string
	Notes      string
	FolderPath string
	CreatedAt  string
	UpdatedAt  string
	CreatedBy  sql.NullInt64
	DeletedAt  sql.NullString
}

// HasFolder reports whether a case-files folder is associated with this client.
func (c Client) HasFolder() bool { return strings.TrimSpace(c.FolderPath) != "" }

// ListClients returns non-deleted clients, optionally filtered by a
// case-insensitive substring match on several fields. Ordered by name.
func ListClients(db *sql.DB, query string) ([]Client, error) {
	const base = `SELECT id, name, address, phone, email, birthday, case_type,
		adversary, notes, folder_path, created_at, updated_at, created_by, deleted_at
		FROM clients WHERE deleted_at IS NULL`

	var rows *sql.Rows
	var err error
	query = strings.TrimSpace(query)
	if query == "" {
		rows, err = db.Query(base + ` ORDER BY name COLLATE NOCASE`)
	} else {
		like := "%" + query + "%"
		rows, err = db.Query(base+` AND (name LIKE ? COLLATE NOCASE
			OR phone LIKE ? COLLATE NOCASE
			OR email LIKE ? COLLATE NOCASE
			OR case_type LIKE ? COLLATE NOCASE
			OR adversary LIKE ? COLLATE NOCASE)
			ORDER BY name COLLATE NOCASE`, like, like, like, like, like)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetClient returns a single non-deleted client by id, or sql.ErrNoRows.
func GetClient(db *sql.DB, id int64) (Client, error) {
	row := db.QueryRow(`SELECT id, name, address, phone, email, birthday, case_type,
		adversary, notes, folder_path, created_at, updated_at, created_by, deleted_at
		FROM clients WHERE id = ? AND deleted_at IS NULL`, id)
	return scanClient(row)
}

// CreateClient inserts a new client and returns its id.
func CreateClient(db *sql.DB, c Client, createdBy int64) (int64, error) {
	res, err := db.Exec(`INSERT INTO clients
		(name, address, phone, email, birthday, case_type, adversary, notes, folder_path, created_by)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.Name, c.Address, c.Phone, c.Email, c.Birthday, c.CaseType,
		c.Adversary, c.Notes, c.FolderPath, createdBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateClient updates the editable fields of an existing client.
func UpdateClient(db *sql.DB, c Client) error {
	_, err := db.Exec(`UPDATE clients SET
		name=?, address=?, phone=?, email=?, birthday=?, case_type=?,
		adversary=?, notes=?, folder_path=?, updated_at=datetime('now')
		WHERE id=? AND deleted_at IS NULL`,
		c.Name, c.Address, c.Phone, c.Email, c.Birthday, c.CaseType,
		c.Adversary, c.Notes, c.FolderPath, c.ID)
	return err
}

// SoftDeleteClient marks a client deleted without removing the row.
func SoftDeleteClient(db *sql.DB, id int64) error {
	_, err := db.Exec(
		`UPDATE clients SET deleted_at=datetime('now') WHERE id=? AND deleted_at IS NULL`,
		id)
	return err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanClient(s scanner) (Client, error) {
	var c Client
	err := s.Scan(&c.ID, &c.Name, &c.Address, &c.Phone, &c.Email, &c.Birthday,
		&c.CaseType, &c.Adversary, &c.Notes, &c.FolderPath, &c.CreatedAt,
		&c.UpdatedAt, &c.CreatedBy, &c.DeletedAt)
	return c, err
}
