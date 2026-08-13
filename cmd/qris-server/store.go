package main

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite" // pure Go, tanpa cgo — `go install` jalan tanpa toolchain C
)

// Store menyimpan QRIS merchant, tagihan, dan notifikasi yang masuk.
type Store struct{ db *sql.DB }

type Notification struct {
	ID          int64  `json:"id"`
	PackageName string `json:"package_name"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	PostedAt    int64  `json:"posted_at"`
	Amount      *int64 `json:"amount"`
	// ChargeID terisi bila notifikasi ini melunasi sebuah tagihan.
	ChargeID   *int64 `json:"charge_id"`
	ReceivedAt int64  `json:"received_at"`
}

// Merchant = satu QRIS statis yang diunggah. ImageFile menunjuk berkas di folder -qris-dir.
type Merchant struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Payload   string `json:"payload"`
	ImageFile string `json:"image_file"`
	CreatedAt int64  `json:"created_at"`
}

// Charge = permintaan bayar bernominal. Amount sudah termasuk kode unik.
type Charge struct {
	ID         int64  `json:"id"`
	MerchantID int64  `json:"merchant_id"`
	BaseAmount int64  `json:"base_amount"`
	UniqueCode int    `json:"unique_code"`
	Amount     int64  `json:"amount"`
	Payload    string `json:"payload"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  int64  `json:"expires_at"`
	PaidAt     *int64 `json:"paid_at"`
}

const (
	ChargePending = "pending"
	ChargePaid    = "paid"
	ChargeExpired = "expired"
)

var ErrNoMerchant = errors.New("belum ada QRIS statis yang diunggah")

func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS merchant (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT    NOT NULL,
			payload    TEXT    NOT NULL UNIQUE,
			image_file TEXT    NOT NULL,
			created_at INTEGER NOT NULL
		);
		CREATE TABLE IF NOT EXISTS charge (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			merchant_id INTEGER NOT NULL REFERENCES merchant(id),
			base_amount INTEGER NOT NULL,
			unique_code INTEGER NOT NULL,
			amount      INTEGER NOT NULL,
			payload     TEXT    NOT NULL,
			status      TEXT    NOT NULL,
			created_at  INTEGER NOT NULL,
			expires_at  INTEGER NOT NULL,
			paid_at     INTEGER
		);
		CREATE INDEX IF NOT EXISTS charge_match ON charge (status, amount);
		CREATE TABLE IF NOT EXISTS notification (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			package_name TEXT    NOT NULL,
			title        TEXT    NOT NULL,
			text         TEXT    NOT NULL,
			posted_at    INTEGER NOT NULL,
			amount       INTEGER,
			charge_id    INTEGER,
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

// ── Merchant ────────────────────────────────────────────────────────────────

func (s *Store) SaveMerchant(m Merchant) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO merchant (name, payload, image_file, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(payload) DO UPDATE SET name = excluded.name, image_file = excluded.image_file`,
		m.Name, m.Payload, m.ImageFile, time.Now().UnixMilli(),
	)
	if err != nil {
		return 0, err
	}
	if id, _ := res.LastInsertId(); id > 0 {
		return id, nil
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM merchant WHERE payload = ?`, m.Payload).Scan(&id)
	return id, err
}

func (s *Store) Merchants() ([]Merchant, error) {
	rows, err := s.db.Query(`SELECT id, name, payload, image_file, created_at FROM merchant ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Merchant{}
	for rows.Next() {
		var m Merchant
		if err := rows.Scan(&m.ID, &m.Name, &m.Payload, &m.ImageFile, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Merchant(id int64) (Merchant, error) {
	var m Merchant
	q := `SELECT id, name, payload, image_file, created_at FROM merchant`
	var err error
	if id > 0 {
		err = s.db.QueryRow(q+` WHERE id = ?`, id).Scan(&m.ID, &m.Name, &m.Payload, &m.ImageFile, &m.CreatedAt)
	} else {
		err = s.db.QueryRow(q + ` ORDER BY id LIMIT 1`).Scan(&m.ID, &m.Name, &m.Payload, &m.ImageFile, &m.CreatedAt)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNoMerchant
	}
	return m, err
}

// ── Charge ──────────────────────────────────────────────────────────────────

// TakenAmounts = nominal tagihan yang masih pending pada rentang kode unik, dipakai uniq.Pick
// supaya dua tagihan tidak pernah punya nominal yang sama.
func (s *Store) TakenAmounts(merchantID, base int64) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT amount FROM charge WHERE merchant_id = ? AND status = ? AND amount >= ?`,
		merchantID, ChargePending, base)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var a int64
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveCharge(c Charge) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO charge (merchant_id, base_amount, unique_code, amount, payload, status, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.MerchantID, c.BaseAmount, c.UniqueCode, c.Amount, c.Payload, ChargePending, c.CreatedAt, c.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Charges(limit int) ([]Charge, error) {
	rows, err := s.db.Query(
		`SELECT id, merchant_id, base_amount, unique_code, amount, payload, status, created_at, expires_at, paid_at
		 FROM charge ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Charge{}
	for rows.Next() {
		var c Charge
		if err := rows.Scan(&c.ID, &c.MerchantID, &c.BaseAmount, &c.UniqueCode, &c.Amount, &c.Payload,
			&c.Status, &c.CreatedAt, &c.ExpiresAt, &c.PaidAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) Charge(id int64) (Charge, error) {
	var c Charge
	err := s.db.QueryRow(
		`SELECT id, merchant_id, base_amount, unique_code, amount, payload, status, created_at, expires_at, paid_at
		 FROM charge WHERE id = ?`, id).
		Scan(&c.ID, &c.MerchantID, &c.BaseAmount, &c.UniqueCode, &c.Amount, &c.Payload,
			&c.Status, &c.CreatedAt, &c.ExpiresAt, &c.PaidAt)
	return c, err
}

// ExpireDue menandai tagihan pending yang sudah lewat waktu. Dipanggil sebelum mencocokkan supaya
// pembayaran telat tidak melunasi tagihan yang seharusnya sudah mati.
func (s *Store) ExpireDue(now int64) error {
	_, err := s.db.Exec(`UPDATE charge SET status = ? WHERE status = ? AND expires_at < ?`,
		ChargeExpired, ChargePending, now)
	return err
}

// MatchCharge mencari tagihan pending bernominal PERSIS sama, yang paling lama menunggu (FIFO),
// lalu menandainya lunas. Return 0 bila tidak ada yang cocok.
func (s *Store) MatchCharge(amount, now int64) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRow(
		`SELECT id FROM charge WHERE status = ? AND amount = ? ORDER BY created_at ASC LIMIT 1`,
		ChargePending, amount).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`UPDATE charge SET status = ?, paid_at = ? WHERE id = ?`, ChargePaid, now, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// ── Notification ────────────────────────────────────────────────────────────

// SaveNotification mengabaikan duplikat — pengiriman ulang harus tetap dijawab 2xx.
func (s *Store) SaveNotification(n Notification) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO notification (package_name, title, text, posted_at, amount, charge_id, received_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.PackageName, n.Title, n.Text, n.PostedAt, n.Amount, n.ChargeID, time.Now().UnixMilli())
	return err
}

func (s *Store) Notifications(limit int) ([]Notification, error) {
	rows, err := s.db.Query(
		`SELECT id, package_name, title, text, posted_at, amount, charge_id, received_at
		 FROM notification ORDER BY posted_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.PackageName, &n.Title, &n.Text, &n.PostedAt, &n.Amount,
			&n.ChargeID, &n.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
