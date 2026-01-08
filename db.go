package dix

import (
	"database/sql"
	"encoding/json"

	_ "modernc.org/sqlite"
)

func Opendb(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS intents (
			id TEXT PRIMARY KEY,
			from_pubkey TEXT,
			to_pubkey TEXT,
			to_resolved TEXT,
			amount INTEGER,
			token TEXT DEFAULT 'usdc',
			signature TEXT,
			time INTEGER,
			status TEXT
		);
		
		CREATE TABLE IF NOT EXISTS aliases (
			username TEXT PRIMARY KEY,
			pubkey TEXT
		);

		CREATE TABLE IF NOT EXISTS pools (
			id TEXT PRIMARY KEY,
			name TEXT,
			token TEXT,
			contribution INTEGER,
			members TEXT,
			round INTEGER,
			created_at INTEGER,
			status TEXT
		);

		CREATE TABLE IF NOT EXISTS pool_members (
			pool_id TEXT,
			username TEXT,
			pubkey TEXT,
			paid INTEGER DEFAULT 0,
			claimed INTEGER DEFAULT 0,
			member_order INTEGER,
			PRIMARY KEY (pool_id, username)
		);
	`)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func Save(db *sql.DB, i Intent) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO intents 
		(id, from_pubkey, to_pubkey, to_resolved, amount, token, signature, time, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, i.ID, i.From, i.To, i.ToResolved, i.Amount, i.Token, i.Signature, i.Time, i.Status)
	return err
}

func Load(db *sql.DB, id string) (Intent, error) {
	var i Intent
	var token sql.NullString
	err := db.QueryRow(`
		SELECT id, from_pubkey, to_pubkey, to_resolved, amount, token, signature, time, status
		FROM intents WHERE id = ?
	`, id).Scan(&i.ID, &i.From, &i.To, &i.ToResolved, &i.Amount, &token, &i.Signature, &i.Time, &i.Status)
	if token.Valid {
		i.Token = token.String
	} else {
		i.Token = "usdc"
	}
	return i, err
}

func List(db *sql.DB, limit int) ([]Intent, error) {
	rows, err := db.Query(`
		SELECT id, from_pubkey, to_pubkey, to_resolved, amount, token, signature, time, status
		FROM intents ORDER BY time DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Intent
	for rows.Next() {
		var i Intent
		var token sql.NullString
		err := rows.Scan(&i.ID, &i.From, &i.To, &i.ToResolved, &i.Amount, &token, &i.Signature, &i.Time, &i.Status)
		if err != nil {
			continue
		}
		if token.Valid {
			i.Token = token.String
		} else {
			i.Token = "usdc"
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func Savealias(db *sql.DB, username, pubkey string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO aliases (username, pubkey) VALUES (?, ?)`, username, pubkey)
	return err
}

func Getalias(db *sql.DB, username string) (string, error) {
	var pubkey string
	err := db.QueryRow(`SELECT pubkey FROM aliases WHERE username = ?`, username).Scan(&pubkey)
	return pubkey, err
}

func Listaliases(db *sql.DB) ([]Alias, error) {
	rows, err := db.Query(`SELECT username, pubkey FROM aliases ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alias
	for rows.Next() {
		var a Alias
		rows.Scan(&a.Username, &a.Owner)
		out = append(out, a)
	}
	return out, rows.Err()
}

func SavePool(db *sql.DB, p Pool) error {
	members, _ := json.Marshal(p.Members)
	_, err := db.Exec(`
		INSERT OR REPLACE INTO pools 
		(id, name, token, contribution, members, round, created_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Token, p.Contribution, string(members), p.Round, p.CreatedAt, p.Status)
	return err
}

func LoadPool(db *sql.DB, id string) (Pool, error) {
	var p Pool
	var members string
	err := db.QueryRow(`
		SELECT id, name, token, contribution, members, round, created_at, status
		FROM pools WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Token, &p.Contribution, &members, &p.Round, &p.CreatedAt, &p.Status)
	if err != nil {
		return Pool{}, err
	}
	json.Unmarshal([]byte(members), &p.Members)
	return p, nil
}

func ListPools(db *sql.DB) ([]Pool, error) {
	rows, err := db.Query(`SELECT id, name, token, contribution, members, round, created_at, status FROM pools ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Pool
	for rows.Next() {
		var p Pool
		var members string
		rows.Scan(&p.ID, &p.Name, &p.Token, &p.Contribution, &members, &p.Round, &p.CreatedAt, &p.Status)
		json.Unmarshal([]byte(members), &p.Members)
		out = append(out, p)
	}
	return out, rows.Err()
}

func AddPoolMember(db *sql.DB, poolID, username, pubkey string, order int) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO pool_members (pool_id, username, pubkey, paid, claimed, member_order)
		VALUES (?, ?, ?, 0, 0, ?)
	`, poolID, username, pubkey, order)
	return err
}

func GetPoolMember(db *sql.DB, poolID, username string) (PoolMember, error) {
	var m PoolMember
	var paid, claimed int
	err := db.QueryRow(`
		SELECT pool_id, username, pubkey, paid, claimed, member_order
		FROM pool_members WHERE pool_id = ? AND username = ?
	`, poolID, username).Scan(&m.PoolID, &m.Username, &m.Pubkey, &paid, &claimed, &m.Order)
	m.Paid = paid == 1
	m.Claimed = claimed == 1
	return m, err
}

func ListPoolMembers(db *sql.DB, poolID string) ([]PoolMember, error) {
	rows, err := db.Query(`
		SELECT pool_id, username, pubkey, paid, claimed, member_order
		FROM pool_members WHERE pool_id = ? ORDER BY member_order
	`, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PoolMember
	for rows.Next() {
		var m PoolMember
		var paid, claimed int
		rows.Scan(&m.PoolID, &m.Username, &m.Pubkey, &paid, &claimed, &m.Order)
		m.Paid = paid == 1
		m.Claimed = claimed == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

func GetRoundWinner(db *sql.DB, poolID string, round int) (PoolMember, error) {
	var m PoolMember
	var paid, claimed int
	err := db.QueryRow(`
		SELECT pool_id, username, pubkey, paid, claimed, member_order
		FROM pool_members WHERE pool_id = ? AND member_order = ?
	`, poolID, round-1).Scan(&m.PoolID, &m.Username, &m.Pubkey, &paid, &claimed, &m.Order)
	m.Paid = paid == 1
	m.Claimed = claimed == 1
	return m, err
}

func MarkPaid(db *sql.DB, poolID, username string) error {
	_, err := db.Exec(`UPDATE pool_members SET paid = 1 WHERE pool_id = ? AND username = ?`, poolID, username)
	return err
}

func MarkClaimed(db *sql.DB, poolID, username string) error {
	_, err := db.Exec(`UPDATE pool_members SET claimed = 1 WHERE pool_id = ? AND username = ?`, poolID, username)
	return err
}

func ResetPaid(db *sql.DB, poolID string) error {
	_, err := db.Exec(`UPDATE pool_members SET paid = 0 WHERE pool_id = ?`, poolID)
	return err
}
