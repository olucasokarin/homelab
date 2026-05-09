package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

type Database struct {
	Conn *sql.DB
}

func Connect(dbPath string) (*Database, error) {
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &Database{Conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	log.Printf("Connected to SQLite database at %s", dbPath)
	return db, nil
}

func (db *Database) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS sniffs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		torrent_name TEXT,
		file_name TEXT,
		file_size INTEGER,
		magnet_link TEXT,
		probe_tool TEXT,
		probe_json TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := db.Conn.Exec(query)
	// Adiciona colunas se não existirem
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN downloaded_bytes INTEGER DEFAULT 0`)
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN tail_fallback BOOLEAN DEFAULT 0`)
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN seeds INTEGER DEFAULT 0`)
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN peers INTEGER DEFAULT 0`)
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN torrent_size INTEGER DEFAULT 0`)
	db.Conn.Exec(`ALTER TABLE sniffs ADD COLUMN notes TEXT DEFAULT ''`)
	return err
}

func (db *Database) SaveSniff(torrentName, fileName string, fileSize, torrentSize int64, magnet, probeTool, probeJson string, downloadedBytes int64, tailFallback bool, seeds, peers int) error {
	query := `INSERT INTO sniffs (torrent_name, file_name, file_size, torrent_size, magnet_link, probe_tool, probe_json, downloaded_bytes, tail_fallback, seeds, peers) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Conn.Exec(query, torrentName, fileName, fileSize, torrentSize, magnet, probeTool, probeJson, downloadedBytes, tailFallback, seeds, peers)
	return err
}

func (db *Database) DeleteSniff(id string) error {
	_, err := db.Conn.Exec("DELETE FROM sniffs WHERE id = ?", id)
	return err
}

func (db *Database) UpdateNotes(id string, notes string) error {
	_, err := db.Conn.Exec("UPDATE sniffs SET notes = ? WHERE id = ?", notes, id)
	return err
}

func (db *Database) FlushSniffs() error {
	_, err := db.Conn.Exec("DELETE FROM sniffs")
	return err
}

func (db *Database) GetHistory(limit int) ([]map[string]interface{}, error) {
	query := `SELECT id, torrent_name, file_name, file_size, torrent_size, magnet_link, probe_tool, probe_json, created_at, downloaded_bytes, tail_fallback, seeds, peers, notes FROM sniffs ORDER BY created_at DESC LIMIT ?`
	rows, err := db.Conn.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var id int
		var torrentName, fileName, magnet, probeTool, probeJson, createdAt string
		var fileSize, torrentSize, downloadedBytes int64
		var tailFallback bool
		var seeds, peers int
		var notes sql.NullString
		if err := rows.Scan(&id, &torrentName, &fileName, &fileSize, &torrentSize, &magnet, &probeTool, &probeJson, &createdAt, &downloadedBytes, &tailFallback, &seeds, &peers, &notes); err != nil {
			return nil, err
		}
		notesStr := ""
		if notes.Valid {
			notesStr = notes.String
		}
		history = append(history, map[string]interface{}{
			"id":               id,
			"torrent_name":     torrentName,
			"file_name":        fileName,
			"file_size":        fileSize,
			"torrent_size":     torrentSize,
			"magnet":           magnet,
			"probe_tool":       probeTool,
			"probe":            probeJson,
			"created_at":       createdAt,
			"downloaded_bytes": downloadedBytes,
			"tail_fallback":    tailFallback,
			"seeds":            seeds,
			"peers":            peers,
			"notes":            notesStr,
		})
	}
	return history, nil
}
