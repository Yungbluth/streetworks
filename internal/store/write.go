package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

const schema = `
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL) WITHOUT ROWID;

CREATE TABLE permits (
  id          INTEGER PRIMARY KEY,
  place       INTEGER NOT NULL,
  year        INTEGER NOT NULL,
  cat         INTEGER NOT NULL,
  number      TEXT NOT NULL,
  type        TEXT NOT NULL,
  sub_type    TEXT,
  status      TEXT NOT NULL,
  no_work     INTEGER NOT NULL,
  date        TEXT NOT NULL,
  date_basis  TEXT NOT NULL,
  final       TEXT,
  description TEXT NOT NULL,
  applicant   TEXT,
  utility     TEXT,
  filed       TEXT NOT NULL,
  link        TEXT,
  located     TEXT,
  flags       TEXT
);
CREATE INDEX permits_number ON permits(number);

CREATE VIRTUAL TABLE permits_fts USING fts5(
  number, description, applicant,
  content='permits', content_rowid='id', tokenize='unicode61'
);
`

func Write(path string, m Meta, ps []Permit) (err error) {
	tmp := path + ".tmp"
	os.Remove(tmp)
	defer func() {
		if err != nil {
			os.Remove(tmp)
		}
	}()

	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return err
	}
	if err := fill(db, m, ps); err != nil {
		db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fill(db *sql.DB, m Meta, ps []Permit) error {
	// one connection, so the transaction and VACUUM see the same file
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range map[string]string{
		"built":   m.Built,
		"version": strconv.Itoa(m.Version),
		"permits": strconv.Itoa(m.Permits),
	} {
		if _, err := tx.Exec("INSERT INTO meta (key, value) VALUES (?, ?)", k, v); err != nil {
			return err
		}
	}

	stmt, err := tx.Prepare(`INSERT INTO permits (id, place, year, cat, number, ` + columns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, p := range ps {
		d := p.Detail
		var flags any
		if len(d.Flags) > 0 {
			b, err := json.Marshal(d.Flags)
			if err != nil {
				return err
			}
			flags = string(b)
		}
		_, err := stmt.Exec(p.ID, p.Place, p.Year, p.Cat, d.Permit, d.Type, null(d.SubType), d.Status,
			d.NoWork, d.Date, d.DateBasis, null(d.Final), d.Description, null(d.Applicant),
			null(d.Utility), d.Filed, null(d.Link), null(d.Located), flags)
		if err != nil {
			return fmt.Errorf("writing permit %s: %w", d.Permit, err)
		}
	}
	if _, err := tx.Exec("INSERT INTO permits_fts(permits_fts) VALUES('rebuild')"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	_, err = db.Exec("VACUUM")
	return err
}

func null(s string) any {
	if s == "" {
		return nil
	}
	return s
}
