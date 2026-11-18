package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "github.com/mattn/go-sqlite3"

	"neuralnetwork/internal/models"
)

// Store gestiona la persistencia ligera en SQLite (sin historial de tráfico).
type Store struct {
	db *sql.DB
}

// NewStore abre o inicializa la base SQLite en dbPath y ejecuta las migraciones requeridas.
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = "/data/neuralnetwork.db"
	}
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("no se pudo crear directorio de datos %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir SQLite en %q: %w", dbPath, err)
	}

	// SQLite soporta mejor concurrencia con una única conexión o WAL + busy_timeout.
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("error ejecutando %q: %w", p, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS known_nodes (
		ip         TEXT PRIMARY KEY,
		mac        TEXT NOT NULL DEFAULT '',
		alias      TEXT NOT NULL DEFAULT '',
		category   TEXT NOT NULL DEFAULT 'default',
		pinned     INTEGER NOT NULL DEFAULT 0,
		fx         REAL,
		fy         REAL,
		fz         REAL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS settings (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error inicializando esquema SQLite: %w", err)
	}

	return &Store{db: db}, nil
}

// Close cierra la conexión SQLite.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// GetKnownNodes devuelve el mapa de personalizaciones guardadas indexado por IP.
func (s *Store) GetKnownNodes() (map[string]models.NodeCustomization, error) {
	rows, err := s.db.Query(`SELECT ip, mac, alias, category, pinned, fx, fy, fz FROM known_nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]models.NodeCustomization)
	for rows.Next() {
		var c models.NodeCustomization
		var pinnedInt int
		var fx, fy, fz sql.NullFloat64
		if err := rows.Scan(&c.IP, &c.MAC, &c.Alias, &c.Category, &pinnedInt, &fx, &fy, &fz); err != nil {
			return nil, err
		}
		c.Pinned = pinnedInt == 1
		if fx.Valid {
			v := fx.Float64
			c.FX = &v
		}
		if fy.Valid {
			v := fy.Float64
			c.FY = &v
		}
		if fz.Valid {
			v := fz.Float64
			c.FZ = &v
		}
		result[c.IP] = c
	}
	return result, rows.Err()
}

// UpsertNodeCustomization inserta o actualiza alias, categoría y coordenadas fijadas de un nodo.
func (s *Store) UpsertNodeCustomization(c models.NodeCustomization) error {
	pinnedInt := 0
	if c.Pinned {
		pinnedInt = 1
	}
	if c.Category == "" {
		c.Category = "default"
	}

	var fx, fy, fz sql.NullFloat64
	if c.FX != nil {
		fx = sql.NullFloat64{Float64: *c.FX, Valid: true}
	}
	if c.FY != nil {
		fy = sql.NullFloat64{Float64: *c.FY, Valid: true}
	}
	if c.FZ != nil {
		fz = sql.NullFloat64{Float64: *c.FZ, Valid: true}
	}

	_, err := s.db.Exec(`
		INSERT INTO known_nodes (ip, mac, alias, category, pinned, fx, fy, fz, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(ip) DO UPDATE SET
			mac        = CASE WHEN excluded.mac <> '' THEN excluded.mac ELSE known_nodes.mac END,
			alias      = excluded.alias,
			category   = excluded.category,
			pinned     = excluded.pinned,
			fx         = excluded.fx,
			fy         = excluded.fy,
			fz         = excluded.fz,
			updated_at = CURRENT_TIMESTAMP
	`, c.IP, c.MAC, c.Alias, c.Category, pinnedInt, fx, fy, fz)
	return err
}

// DeleteNodeCustomization elimina la personalización de un nodo por su IP.
func (s *Store) DeleteNodeCustomization(ip string) error {
	_, err := s.db.Exec(`DELETE FROM known_nodes WHERE ip = ?`, ip)
	return err
}

// GetSettings recupera preferencias globales combinándolas con valores por defecto.
func (s *Store) GetSettings(defaults models.AppSettings) (models.AppSettings, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return defaults, err
	}
	defer rows.Close()

	out := defaults
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return out, err
		}
		switch k {
		case "groupExternal":
			if b, err := strconv.ParseBool(v); err == nil {
				out.GroupExternal = b
			}
		case "nodeTtlSec":
			if n, err := strconv.Atoi(v); err == nil && n >= 10 {
				out.NodeTTLSec = n
			}
		case "bloomStrength":
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				out.BloomStrength = f
			}
		case "captureMode":
			if v == "live" || v == "simulation" {
				out.CaptureMode = v
			}
		case "interface":
			if v != "" {
				out.Interface = v
			}
		}
	}
	return out, rows.Err()
}

// SaveSettings persiste las preferencias globales en SQLite.
func (s *Store) SaveSettings(settings models.AppSettings) error {
	pairs := map[string]string{
		"groupExternal": strconv.FormatBool(settings.GroupExternal),
		"nodeTtlSec":    strconv.Itoa(settings.NodeTTLSec),
		"bloomStrength": strconv.FormatFloat(settings.BloomStrength, 'f', 2, 64),
		"captureMode":   settings.CaptureMode,
		"interface":     settings.Interface,
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmt.Close()

	for k, v := range pairs {
		if _, err := stmt.Exec(k, v); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
