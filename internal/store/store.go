// package store keeps permit records in a SQLite file
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// Detail is one permit's record as the page shows it
type Detail struct {
	Permit      string   `json:"permit"`
	Type        string   `json:"type"`
	SubType     string   `json:"subType,omitempty"`
	Status      string   `json:"status"`
	NoWork      bool     `json:"noWork"`
	Date        string   `json:"date"`
	DateBasis   string   `json:"dateBasis"`
	Final       string   `json:"final,omitempty"`
	Description string   `json:"description"`
	Applicant   string   `json:"applicant,omitempty"`
	Utility     string   `json:"utility,omitempty"`
	Filed       string   `json:"filed"`
	Link        string   `json:"link,omitempty"`
	Located     string   `json:"located,omitempty"` // how the city placed the point, e.g. "Mapguide Location"
	Flags       []string `json:"flags,omitempty"`
}

type Permit struct {
	ID     int // the permit's index in permits.json
	Place  int // index into places.json
	Year   int // 0 when there's no usable date
	Cat    int // index into the index.json categories
	Detail Detail
}

type Meta struct {
	Built   string `json:"built"`
	Version int    `json:"version"`
	Permits int    `json:"permits"`
}

var ErrNotFound = errors.New("not found")

// DB is a read-only handle on a database written by Write
type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s doesn't exist; run streetworks build first", path)
		}
		return nil, err
	}
	// immutable lets SQLite skip locking
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) Meta(ctx context.Context) (Meta, error) {
	rows, err := d.db.QueryContext(ctx, "SELECT key, value FROM meta")
	if err != nil {
		return Meta{}, err
	}
	defer rows.Close()
	var m Meta
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Meta{}, err
		}
		switch k {
		case "built":
			m.Built = v
		case "version", "permits":
			n, err := strconv.Atoi(v)
			if err != nil {
				return Meta{}, fmt.Errorf("meta %s is %q, not a number", k, v)
			}
			if k == "version" {
				m.Version = n
			} else {
				m.Permits = n
			}
		}
	}
	return m, rows.Err()
}

const columns = `type, sub_type, status, no_work, date, date_basis, final, description,
	applicant, utility, filed, link, located, flags`

// Lookup returns the records for ids, keyed by id
func (d *DB) Lookup(ctx context.Context, ids []int) (map[int]Detail, error) {
	out := make(map[int]Detail, len(ids))
	seen := make(map[int]bool, len(ids))
	var args []any
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			args = append(args, id)
		}
	}
	if len(args) == 0 {
		return out, nil
	}
	q := "SELECT id, number, " + columns + " FROM permits WHERE id IN (?" +
		strings.Repeat(",?", len(args)-1) + ")"
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var number string
		det, err := scanRow(rows, &id, &number)
		if err != nil {
			return nil, err
		}
		det.Permit = number
		out[id] = det
	}
	return out, rows.Err()
}

func (d *DB) ByNumber(ctx context.Context, number string) (Permit, error) {
	q := "SELECT id, place, year, cat, number, " + columns +
		" FROM permits WHERE number = upper(trim(?))"
	var p Permit
	var n string
	det, err := scanRow(d.db.QueryRowContext(ctx, q, number), &p.ID, &p.Place, &p.Year, &p.Cat, &n)
	if errors.Is(err, sql.ErrNoRows) {
		return Permit{}, ErrNotFound
	}
	if err != nil {
		return Permit{}, err
	}
	det.Permit = n
	p.Detail = det
	return p, nil
}

// scanRow scans lead into the first columns and the detail columns after them
func scanRow(s interface{ Scan(...any) error }, lead ...any) (Detail, error) {
	var subType, final, applicant, utility, link, located, flags sql.NullString
	var d Detail
	dest := append(lead, &d.Type, &subType, &d.Status, &d.NoWork, &d.Date, &d.DateBasis, &final,
		&d.Description, &applicant, &utility, &d.Filed, &link, &located, &flags)
	if err := s.Scan(dest...); err != nil {
		return Detail{}, err
	}
	d.SubType, d.Final, d.Applicant = subType.String, final.String, applicant.String
	d.Utility, d.Link, d.Located = utility.String, link.String, located.String
	if flags.Valid {
		if err := json.Unmarshal([]byte(flags.String), &d.Flags); err != nil {
			return Detail{}, fmt.Errorf("reading flags: %w", err)
		}
	}
	return d, nil
}
