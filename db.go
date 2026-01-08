package dix

import (
	"database/sql"

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
