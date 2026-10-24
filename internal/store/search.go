package store

import (
	"context"
	"errors"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Hit is one search result
type Hit struct {
	ID      int    `json:"id"`
	Place   int    `json:"place"`
	Year    int    `json:"year"`
	Cat     int    `json:"cat"`
	Number  string `json:"number"`
	Snippet string `json:"snippet"` // HTML-safe, with matches in <mark>
}

// ErrBadQuery marks a search the user can fix by typing something else
var ErrBadQuery = errors.New("bad query")

type badQuery string

func (e badQuery) Error() string        { return string(e) }
func (e badQuery) Is(target error) bool { return target == ErrBadQuery }

const (
	maxQueryChars = 100
	defaultLimit  = 20
	maxLimit      = 50
)

// MatchQuery turns what someone typed into an FTS5 MATCH expression
func MatchQuery(q string) (string, error) {
	if utf8.RuneCountInString(q) > maxQueryChars {
		return "", badQuery("Searches can be up to 100 characters.")
	}
	// only letters and digits get through, so nobody can write their own FTS syntax
	words := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if utf8.RuneCountInString(strings.Join(words, "")) < 2 {
		return "", badQuery("Type at least 2 letters or digits.")
	}
	return `"` + strings.Join(words, `" "`) + `"*`, nil
}

func clampLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	return min(n, maxLimit)
}

func (d *DB) Search(ctx context.Context, q string, limit int) ([]Hit, error) {
	match, err := MatchQuery(q)
	if err != nil {
		return nil, err
	}
	// The snippet marks matches with control characters, which can't occur in the text
	rows, err := d.db.QueryContext(ctx, `
		SELECT p.id, p.place, p.year, p.cat, p.number,
		       snippet(permits_fts, -1, char(1), char(2), '…', 12)
		FROM permits_fts JOIN permits p ON p.id = permits_fts.rowid
		WHERE permits_fts MATCH ?
		ORDER BY bm25(permits_fts), p.id
		LIMIT ?`, match, clampLimit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	marks := strings.NewReplacer("\x01", "<mark>", "\x02", "</mark>")
	hits := []Hit{}
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Place, &h.Year, &h.Cat, &h.Number, &h.Snippet); err != nil {
			return nil, err
		}
		h.Snippet = marks.Replace(html.EscapeString(h.Snippet))
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
