package profile

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers "sqlite" driver
)

// ---------------------------------------------------------------------------
// CookieStore — SQLite-backed persistent cookie storage
// ---------------------------------------------------------------------------

const createCookiesTable = `
CREATE TABLE IF NOT EXISTS cookies (
    domain    TEXT NOT NULL,
    name      TEXT NOT NULL,
    value     TEXT NOT NULL,
    expires   DATETIME,
    http_only INTEGER DEFAULT 0,
    secure    INTEGER DEFAULT 0,
    PRIMARY KEY (domain, name)
);`

// CookieStore persists HTTP cookies for a profile in a SQLite database.
type CookieStore struct {
	db *sql.DB
}

// NewCookieStore opens (or creates) the SQLite database at dbPath and
// ensures the cookies table exists.
func NewCookieStore(dbPath string) (*CookieStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cookiestore: open db %q: %w", dbPath, err)
	}

	if _, err := db.Exec(createCookiesTable); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cookiestore: create table: %w", err)
	}

	return &CookieStore{db: db}, nil
}

// Set inserts or replaces a cookie in the store.
func (s *CookieStore) Set(domain, name, value string, expires time.Time, httpOnly, secure bool) error {
	var expiresVal interface{}
	if !expires.IsZero() {
		expiresVal = expires.UTC().Format(time.RFC3339)
	}

	httpOnlyInt := 0
	if httpOnly {
		httpOnlyInt = 1
	}
	secureInt := 0
	if secure {
		secureInt = 1
	}

	_, err := s.db.Exec(
		`INSERT INTO cookies (domain, name, value, expires, http_only, secure)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(domain, name) DO UPDATE SET
		     value     = excluded.value,
		     expires   = excluded.expires,
		     http_only = excluded.http_only,
		     secure    = excluded.secure`,
		domain, name, value, expiresVal, httpOnlyInt, secureInt,
	)
	if err != nil {
		return fmt.Errorf("cookiestore: set %q/%q: %w", domain, name, err)
	}
	return nil
}

// Get returns all cookies for domain.
func (s *CookieStore) Get(domain string) ([]*http.Cookie, error) {
	rows, err := s.db.Query(
		`SELECT name, value, expires, http_only, secure
		 FROM cookies WHERE domain = ?`, domain,
	)
	if err != nil {
		return nil, fmt.Errorf("cookiestore: get %q: %w", domain, err)
	}
	defer rows.Close()

	var cookies []*http.Cookie
	for rows.Next() {
		var (
			name, value    string
			expiresStr     sql.NullString
			httpOnlyInt    int
			secureInt      int
		)
		if err := rows.Scan(&name, &value, &expiresStr, &httpOnlyInt, &secureInt); err != nil {
			return nil, fmt.Errorf("cookiestore: scan row: %w", err)
		}

		c := &http.Cookie{
			Name:     name,
			Value:    value,
			Domain:   domain,
			HttpOnly: httpOnlyInt != 0,
			Secure:   secureInt != 0,
		}

		if expiresStr.Valid && expiresStr.String != "" {
			t, err := time.Parse(time.RFC3339, expiresStr.String)
			if err == nil {
				c.Expires = t
			}
		}

		cookies = append(cookies, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cookiestore: rows error: %w", err)
	}

	return cookies, nil
}

// Delete removes a specific cookie by domain and name.
func (s *CookieStore) Delete(domain, name string) error {
	_, err := s.db.Exec(`DELETE FROM cookies WHERE domain = ? AND name = ?`, domain, name)
	if err != nil {
		return fmt.Errorf("cookiestore: delete %q/%q: %w", domain, name, err)
	}
	return nil
}

// Clear removes all cookies from the store.
func (s *CookieStore) Clear() error {
	_, err := s.db.Exec(`DELETE FROM cookies`)
	if err != nil {
		return fmt.Errorf("cookiestore: clear: %w", err)
	}
	return nil
}

// Close closes the underlying database connection.
func (s *CookieStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// LocalStorage — JSON-file-backed localStorage per origin
// ---------------------------------------------------------------------------

// LocalStorage implements a simple key-value store scoped to origins,
// backed by one JSON file per origin in a directory.
type LocalStorage struct {
	dir string
}

// NewLocalStorage returns a LocalStorage backed by dir.
func NewLocalStorage(dir string) *LocalStorage {
	return &LocalStorage{dir: dir}
}

// originFile returns the path to the JSON file for a given origin.
// The origin string is sanitised to be a safe filename.
func (ls *LocalStorage) originFile(origin string) string {
	safe := strings.NewReplacer(
		"/", "_",
		":", "_",
		"\\", "_",
	).Replace(origin)
	return filepath.Join(ls.dir, safe+".json")
}

// readOrigin reads the key-value map for an origin from disk.
// Returns an empty map if the file does not exist.
func (ls *LocalStorage) readOrigin(origin string) (map[string]string, error) {
	path := ls.originFile(origin)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("localstorage: read %q: %w", path, err)
	}

	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("localstorage: parse %q: %w", path, err)
	}
	return m, nil
}

// writeOrigin persists the key-value map for an origin to disk.
func (ls *LocalStorage) writeOrigin(origin string, m map[string]string) error {
	if err := os.MkdirAll(ls.dir, 0o755); err != nil {
		return fmt.Errorf("localstorage: mkdir %q: %w", ls.dir, err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("localstorage: marshal %q: %w", origin, err)
	}

	path := ls.originFile(origin)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("localstorage: write tmp %q: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("localstorage: rename %q: %w", path, err)
	}
	return nil
}

// Get retrieves the value for key in the given origin's storage.
// Returns "" and no error if the key is absent.
func (ls *LocalStorage) Get(origin, key string) (string, error) {
	m, err := ls.readOrigin(origin)
	if err != nil {
		return "", err
	}
	return m[key], nil
}

// Set sets key to value in the given origin's storage.
func (ls *LocalStorage) Set(origin, key, value string) error {
	m, err := ls.readOrigin(origin)
	if err != nil {
		return err
	}
	m[key] = value
	return ls.writeOrigin(origin, m)
}

// Remove deletes a single key from the given origin's storage.
func (ls *LocalStorage) Remove(origin, key string) error {
	m, err := ls.readOrigin(origin)
	if err != nil {
		return err
	}
	delete(m, key)
	return ls.writeOrigin(origin, m)
}

// Clear removes all keys for the given origin (deletes its JSON file).
func (ls *LocalStorage) Clear(origin string) error {
	path := ls.originFile(origin)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("localstorage: clear %q: %w", origin, err)
	}
	return nil
}
