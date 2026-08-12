package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite" // pure Go, tanpa cgo — `go install` jalan tanpa toolchain C
)

// Store menyimpan notifikasi yang masuk ke SQLite. Dedup lewat unique index
// (package_name, text, posted_at): aplikasi Android mengirim ulang notifikasi yang gagal, dan
// menyapu ulang shade tiap rebind.
type Store struct{ db *sql.DB }

type Notification struct {
	ID          int64  `json:"id"`
	PackageName string `json:"package_name"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	PostedAt    int64  `json:"posted_at"`
	Amount      *int64 `json:"amount"`
	ReceivedAt  int64  `json:"received_at"`
}

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS notification (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			package_name TEXT    NOT NULL,
			title        TEXT    NOT NULL,
			text         TEXT    NOT NULL,
			posted_at    INTEGER NOT NULL,
			amount       INTEGER,
			received_at  INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS notification_dedup
			ON notification (package_name, text, posted_at);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Save mengabaikan duplikat, bukan menolaknya — pengiriman ulang harus tetap dijawab 2xx.
func (s *Store) Save(n Notification) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO notification (package_name, title, text, posted_at, amount, received_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.PackageName, n.Title, n.Text, n.PostedAt, n.Amount, time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) List(limit int) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, package_name, title, text, posted_at, amount, received_at
		 FROM notification ORDER BY posted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.PackageName, &n.Title, &n.Text, &n.PostedAt, &n.Amount, &n.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
