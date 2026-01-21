package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog"

	"github.com/afroash/dht-monitor/internal/models"
)

// Store defines the interface for sensor data storage
type Store interface {
	Close() error
	Migrate() error
	InsertReading(reading *models.Reading) error
	InsertBatch(readings []*models.Reading) error
	GetReadingsInRange(sensorID string, start, end time.Time, limit int) ([]*models.Reading, error)
	GetReadingsBefore(sensorID string, before time.Time, limit int) ([]*models.Reading, error)
	GetReadingsAfter(sensorID string, after time.Time, limit int) ([]*models.Reading, error)
	GetLatestReading(sensorID string) (*models.Reading, error)
	GetDailyStats(sensorID string, start, end time.Time) ([]DailyStat, error)
	DeleteOlderThan(days int) (int64, error)
	GetStorageStats() (*StorageStats, error)
	GetSensorIDs() ([]string, error)
}

// Compile-time interface check
var _ Store = (*SQLiteStore)(nil)

// SQLiteStore handles persistent storage of sensor readings
type SQLiteStore struct {
	db     *sql.DB
	logger zerolog.Logger
}

// DailyStat represents aggregated statistics for a single day
type DailyStat struct {
	Date           time.Time `json:"date"`
	SensorID       string    `json:"sensor_id"`
	MinTemperature float64   `json:"min_temperature"`
	MaxTemperature float64   `json:"max_temperature"`
	AvgTemperature float64   `json:"avg_temperature"`
	MinHumidity    float64   `json:"min_humidity"`
	MaxHumidity    float64   `json:"max_humidity"`
	AvgHumidity    float64   `json:"avg_humidity"`
	ReadingCount   int       `json:"reading_count"`
}

// StorageStats contains information about the database
type StorageStats struct {
	TotalReadings  int64     `json:"total_readings"`
	OldestReading  time.Time `json:"oldest_reading,omitempty"`
	NewestReading  time.Time `json:"newest_reading,omitempty"`
	UniqueSensors  int       `json:"unique_sensors"`
	DatabaseSizeMB float64   `json:"database_size_mb"`
}

// NewSQLiteStore creates a new SQLite store instance
func NewSQLiteStore(dbPath string, logger zerolog.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Apply performance pragmas for SQLite
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA cache_size=10000",
		"PRAGMA temp_store=MEMORY",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to set pragma %q: %w", pragma, err)
		}
	}

	// Configure connection pool for SQLite (single writer)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	store := &SQLiteStore{
		db:     db,
		logger: logger,
	}

	// Auto-migrate schema
	if err := store.Migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info().Str("path", dbPath).Msg("SQLite store initialized")

	return store, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// Migrate creates the database schema if it doesn't exist
func (s *SQLiteStore) Migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS readings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sensor_id TEXT NOT NULL,
		temperature REAL NOT NULL,
		humidity REAL NOT NULL,
		recorded_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_readings_sensor_time ON readings(sensor_id, recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_readings_time ON readings(recorded_at DESC);
	CREATE INDEX IF NOT EXISTS idx_readings_created ON readings(created_at);
	`

	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	s.logger.Debug().Msg("Database schema migrated")
	return nil
}

// InsertReading inserts a single reading into the database
func (s *SQLiteStore) InsertReading(reading *models.Reading) error {
	query := `
		INSERT INTO readings (sensor_id, temperature, humidity, recorded_at)
		VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		reading.SensorID,
		reading.Temperature,
		reading.Humidity,
		reading.Timestamp.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return fmt.Errorf("failed to insert reading: %w", err)
	}

	return nil
}

// InsertBatch inserts multiple readings in a single transaction
func (s *SQLiteStore) InsertBatch(readings []*models.Reading) error {
	if len(readings) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO readings (sensor_id, temperature, humidity, recorded_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, reading := range readings {
		_, err := stmt.Exec(
			reading.SensorID,
			reading.Temperature,
			reading.Humidity,
			reading.Timestamp.Format("2006-01-02 15:04:05"),
		)
		if err != nil {
			return fmt.Errorf("failed to insert reading in batch: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Debug().Int("count", len(readings)).Msg("Batch insert completed")
	return nil
}

// GetReadingsInRange returns readings within a time range
func (s *SQLiteStore) GetReadingsInRange(sensorID string, start, end time.Time, limit int) ([]*models.Reading, error) {
	var query string
	var args []interface{}

	if sensorID == "" {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE recorded_at BETWEEN ? AND ?
			ORDER BY recorded_at DESC
			LIMIT ?
		`
		args = []interface{}{
			start.Format("2006-01-02 15:04:05"),
			end.Format("2006-01-02 15:04:05"),
			limit,
		}
	} else {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE sensor_id = ? AND recorded_at BETWEEN ? AND ?
			ORDER BY recorded_at DESC
			LIMIT ?
		`
		args = []interface{}{
			sensorID,
			start.Format("2006-01-02 15:04:05"),
			end.Format("2006-01-02 15:04:05"),
			limit,
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings: %w", err)
	}
	defer rows.Close()

	return s.scanReadings(rows)
}

// GetReadingsBefore returns readings before a specific time (for scrolling back)
func (s *SQLiteStore) GetReadingsBefore(sensorID string, before time.Time, limit int) ([]*models.Reading, error) {
	var query string
	var args []interface{}

	if sensorID == "" {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE recorded_at < ?
			ORDER BY recorded_at DESC
			LIMIT ?
		`
		args = []interface{}{
			before.Format("2006-01-02 15:04:05"),
			limit,
		}
	} else {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE sensor_id = ? AND recorded_at < ?
			ORDER BY recorded_at DESC
			LIMIT ?
		`
		args = []interface{}{
			sensorID,
			before.Format("2006-01-02 15:04:05"),
			limit,
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings: %w", err)
	}
	defer rows.Close()

	return s.scanReadings(rows)
}

// GetReadingsAfter returns readings after a specific time (for scrolling forward)
func (s *SQLiteStore) GetReadingsAfter(sensorID string, after time.Time, limit int) ([]*models.Reading, error) {
	var query string
	var args []interface{}

	if sensorID == "" {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE recorded_at > ?
			ORDER BY recorded_at ASC
			LIMIT ?
		`
		args = []interface{}{
			after.Format("2006-01-02 15:04:05"),
			limit,
		}
	} else {
		query = `
			SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
			FROM readings
			WHERE sensor_id = ? AND recorded_at > ?
			ORDER BY recorded_at ASC
			LIMIT ?
		`
		args = []interface{}{
			sensorID,
			after.Format("2006-01-02 15:04:05"),
			limit,
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query readings: %w", err)
	}
	defer rows.Close()

	readings, err := s.scanReadings(rows)
	if err != nil {
		return nil, err
	}

	// Reverse to return newest first (for consistency with other methods)
	for i, j := 0, len(readings)-1; i < j; i, j = i+1, j-1 {
		readings[i], readings[j] = readings[j], readings[i]
	}

	return readings, nil
}

// GetLatestReading returns the most recent reading for a sensor
func (s *SQLiteStore) GetLatestReading(sensorID string) (*models.Reading, error) {
	query := `
		SELECT id, sensor_id, temperature, humidity, recorded_at, created_at
		FROM readings
		WHERE sensor_id = ?
		ORDER BY recorded_at DESC
		LIMIT 1
	`

	row := s.db.QueryRow(query, sensorID)
	reading, err := s.scanReading(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest reading: %w", err)
	}

	return reading, nil
}

// GetDailyStats returns aggregated daily statistics for a time range
func (s *SQLiteStore) GetDailyStats(sensorID string, start, end time.Time) ([]DailyStat, error) {
	var query string
	var args []interface{}

	if sensorID == "" {
		query = `
			SELECT 
				date(recorded_at) as date,
				sensor_id,
				MIN(temperature) as min_temp,
				MAX(temperature) as max_temp,
				AVG(temperature) as avg_temp,
				MIN(humidity) as min_humidity,
				MAX(humidity) as max_humidity,
				AVG(humidity) as avg_humidity,
				COUNT(*) as reading_count
			FROM readings
			WHERE recorded_at BETWEEN ? AND ?
			GROUP BY date(recorded_at), sensor_id
			ORDER BY date DESC
		`
		args = []interface{}{
			start.Format("2006-01-02 15:04:05"),
			end.Format("2006-01-02 15:04:05"),
		}
	} else {
		query = `
			SELECT 
				date(recorded_at) as date,
				sensor_id,
				MIN(temperature) as min_temp,
				MAX(temperature) as max_temp,
				AVG(temperature) as avg_temp,
				MIN(humidity) as min_humidity,
				MAX(humidity) as max_humidity,
				AVG(humidity) as avg_humidity,
				COUNT(*) as reading_count
			FROM readings
			WHERE sensor_id = ? AND recorded_at BETWEEN ? AND ?
			GROUP BY date(recorded_at), sensor_id
			ORDER BY date DESC
		`
		args = []interface{}{
			sensorID,
			start.Format("2006-01-02 15:04:05"),
			end.Format("2006-01-02 15:04:05"),
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily stats: %w", err)
	}
	defer rows.Close()

	var stats []DailyStat
	for rows.Next() {
		var stat DailyStat
		var dateStr string

		err := rows.Scan(
			&dateStr,
			&stat.SensorID,
			&stat.MinTemperature,
			&stat.MaxTemperature,
			&stat.AvgTemperature,
			&stat.MinHumidity,
			&stat.MaxHumidity,
			&stat.AvgHumidity,
			&stat.ReadingCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily stat: %w", err)
		}

		stat.Date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse date: %w", err)
		}

		stats = append(stats, stat)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return stats, nil
}

// DeleteOlderThan removes readings older than the specified number of days
// Note: Deletes based on recorded_at (sensor timestamp), not created_at (insert time)
func (s *SQLiteStore) DeleteOlderThan(days int) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -days)

	result, err := s.db.Exec(
		"DELETE FROM readings WHERE recorded_at < ?",
		cutoff.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old readings: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	s.logger.Info().
		Int("days", days).
		Int64("deleted", deleted).
		Time("cutoff", cutoff).
		Msg("Deleted old readings")

	return deleted, nil
}

// GetStorageStats returns statistics about the database
func (s *SQLiteStore) GetStorageStats() (*StorageStats, error) {
	stats := &StorageStats{}

	// Get total readings count
	err := s.db.QueryRow("SELECT COUNT(*) FROM readings").Scan(&stats.TotalReadings)
	if err != nil {
		return nil, fmt.Errorf("failed to count readings: %w", err)
	}

	// If no readings, return early with zero values
	if stats.TotalReadings == 0 {
		return stats, nil
	}

	// Get oldest and newest reading timestamps
	var oldestStr, newestStr string
	err = s.db.QueryRow("SELECT MIN(recorded_at), MAX(recorded_at) FROM readings").
		Scan(&oldestStr, &newestStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get timestamp range: %w", err)
	}

	stats.OldestReading, _ = s.parseTimestamp(oldestStr)
	stats.NewestReading, _ = s.parseTimestamp(newestStr)

	// Get unique sensor count
	err = s.db.QueryRow("SELECT COUNT(DISTINCT sensor_id) FROM readings").Scan(&stats.UniqueSensors)
	if err != nil {
		return nil, fmt.Errorf("failed to count sensors: %w", err)
	}

	// Get database size using PRAGMA
	var pageCount, pageSize int64
	s.db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	s.db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	stats.DatabaseSizeMB = float64(pageCount*pageSize) / (1024 * 1024)

	return stats, nil
}

// GetSensorIDs returns a list of all unique sensor IDs in the database
func (s *SQLiteStore) GetSensorIDs() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT sensor_id FROM readings ORDER BY sensor_id")
	if err != nil {
		return nil, fmt.Errorf("failed to query sensor IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan sensor ID: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return ids, nil
}

// scanReading is a helper to scan a row into a Reading struct
func (s *SQLiteStore) scanReading(row interface{ Scan(...interface{}) error }) (*models.Reading, error) {
	var r models.Reading
	var id int64
	var recordedAt, createdAt string

	err := row.Scan(&id, &r.SensorID, &r.Temperature, &r.Humidity, &recordedAt, &createdAt)
	if err != nil {
		return nil, err
	}

	r.Timestamp, err = s.parseTimestamp(recordedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse recorded_at: %w", err)
	}

	return &r, nil
}

// scanReadings scans multiple rows into a slice of readings
func (s *SQLiteStore) scanReadings(rows *sql.Rows) ([]*models.Reading, error) {
	var readings []*models.Reading

	for rows.Next() {
		var r models.Reading
		var id int64
		var recordedAt, createdAt string

		err := rows.Scan(&id, &r.SensorID, &r.Temperature, &r.Humidity, &recordedAt, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan reading: %w", err)
		}

		r.Timestamp, err = s.parseTimestamp(recordedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timestamp: %w", err)
		}

		readings = append(readings, &r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return readings, nil
}

// parseTimestamp tries multiple formats to parse a SQLite timestamp
func (s *SQLiteStore) parseTimestamp(ts string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05.000",
		time.RFC3339,
		time.RFC3339Nano,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, ts); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", ts)
}
