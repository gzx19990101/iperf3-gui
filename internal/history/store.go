package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"iperf3-gui/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建历史目录失败: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开历史数据库失败: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("设置数据库参数失败: %w", err)
		}
	}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("读取数据库版本失败: %w", err)
	}
	if version >= 1 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始数据库迁移失败: %w", err)
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			created_at TEXT NOT NULL,
			started_at TEXT,
			ended_at TEXT,
			status TEXT NOT NULL,
			protocol TEXT NOT NULL,
			direction TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER NOT NULL,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			config_json TEXT NOT NULL,
			output_mode TEXT NOT NULL DEFAULT '',
			parser_version TEXT NOT NULL DEFAULT '',
			exit_code INTEGER,
			summary_json TEXT,
			stdout_text TEXT,
			stdout_truncated INTEGER NOT NULL DEFAULT 0,
			stderr_summary TEXT,
			stderr_truncated INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS intervals (
			run_id TEXT NOT NULL,
			sequence INTEGER NOT NULL,
			direction TEXT NOT NULL,
			start_seconds REAL NOT NULL,
			end_seconds REAL NOT NULL,
			bits_per_second REAL NOT NULL,
			bytes INTEGER NOT NULL DEFAULT 0,
			tcp_retransmits INTEGER NOT NULL DEFAULT 0,
			udp_jitter_ms REAL NOT NULL DEFAULT 0,
			udp_lost INTEGER NOT NULL DEFAULT 0,
			udp_total INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (run_id, sequence, direction),
			FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_created ON runs(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_intervals_run ON intervals(run_id, sequence)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("执行数据库迁移失败: %w", err)
		}
	}
	if _, err := tx.Exec("PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("写入数据库版本失败: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交数据库迁移失败: %w", err)
	}
	return nil
}

func (s *Store) MarkStaleRunning() (int, error) {
	result, err := s.db.Exec(
		`UPDATE runs SET status = ?, ended_at = ? WHERE status IN (?, ?, ?)`,
		model.RunStatusInterrupted,
		nowText(),
		model.RunStatusStarting,
		model.RunStatusRunning,
		model.RunStatusStopping,
	)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

func (s *Store) CreateRun(run model.RunRecord) error {
	configJSON, err := json.Marshal(run.Config)
	if err != nil {
		return err
	}
	if run.CreatedAt == "" {
		run.CreatedAt = nowText()
	}
	_, err = s.db.Exec(
		`INSERT INTO runs (
			id, created_at, started_at, ended_at, status, protocol, direction,
			host, port, duration_seconds, config_json, output_mode, parser_version,
			exit_code, summary_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.CreatedAt, nullableText(run.StartedAt), nullableText(run.EndedAt),
		run.Status, run.Protocol, run.Direction, run.Host, run.Port,
		run.Config.DurationSeconds, string(configJSON), run.OutputMode, run.ParserVersion,
		nullableInt(run.ExitCode), nullableSummary(run.Summary),
	)
	return err
}

func (s *Store) AppendIntervals(runID string, intervals []model.Interval) error {
	if len(intervals) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`INSERT OR REPLACE INTO intervals (
		run_id, sequence, direction, start_seconds, end_seconds,
		bits_per_second, bytes, tcp_retransmits,
		udp_jitter_ms, udp_lost, udp_total
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, item := range intervals {
		if _, err := statement.Exec(
			runID, item.Sequence, item.Direction, item.StartSeconds, item.EndSeconds,
			item.BitsPerSecond, item.Bytes, item.TCPRetransmits,
			item.UDPJitterMilliseconds, item.UDPLostDatagrams, item.UDPTotalDatagrams,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) FinishRun(runID string, status model.RunStatus, exitCode *int, summary *model.Summary, stdout string, stdoutTruncated bool, stderr string, stderrTruncated bool) error {
	_, err := s.db.Exec(
		`UPDATE runs SET status = ?, ended_at = ?, exit_code = ?, summary_json = ?,
			stdout_text = ?, stdout_truncated = ?, stderr_summary = ?, stderr_truncated = ?
		WHERE id = ?`,
		status,
		nowText(),
		nullableInt(exitCode),
		nullableSummary(summary),
		stdout,
		boolInt(stdoutTruncated),
		stderr,
		boolInt(stderrTruncated),
		runID,
	)
	return err
}

func (s *Store) List(filter model.HistoryFilter) (model.HistoryPage, error) {
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	query := strings.TrimSpace(filter.Query)
	protocol := strings.ToLower(strings.TrimSpace(filter.Protocol))
	if protocol == "" {
		protocol = "all"
	}
	if protocol != "all" && protocol != "tcp" && protocol != "udp" {
		protocol = "all"
	}
	direction := strings.ToLower(strings.TrimSpace(filter.Direction))
	if direction == "" {
		direction = "all"
	}
	if direction != "all" && direction != "upload" && direction != "download" && direction != "bidirectional" {
		direction = "all"
	}

	where := []string{"1=1"}
	args := make([]interface{}, 0, 3)
	if query != "" {
		where = append(where, "host LIKE ?")
		args = append(args, "%"+query+"%")
	}
	if protocol != "all" {
		where = append(where, "protocol = ?")
		args = append(args, protocol)
	}
	if direction != "all" {
		where = append(where, "direction = ?")
		args = append(args, direction)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM runs WHERE "+whereSQL, args...).Scan(&total); err != nil {
		return model.HistoryPage{}, err
	}

	queryArgs := append(append([]interface{}{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.Query(
		`SELECT id, created_at, started_at, ended_at, status, protocol, direction,
			host, port, duration_seconds, summary_json, exit_code
		FROM runs WHERE `+whereSQL+`
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return model.HistoryPage{}, err
	}
	defer rows.Close()

	items := make([]model.HistoryItem, 0, pageSize)
	for rows.Next() {
		var item model.HistoryItem
		var startedAt, endedAt, summaryJSON sql.NullString
		var exitCode sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.CreatedAt, &startedAt, &endedAt, &item.Status,
			&item.Protocol, &item.Direction, &item.Host, &item.Port,
			&item.DurationSeconds, &summaryJSON, &exitCode,
		); err != nil {
			return model.HistoryPage{}, err
		}
		item.StartedAt = startedAt.String
		item.EndedAt = endedAt.String
		if exitCode.Valid {
			value := int(exitCode.Int64)
			item.ExitCode = &value
		}
		item.UploadBitsPerSecond, item.DownloadBitsPerSecond = summaryBPS(summaryJSON.String)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return model.HistoryPage{}, err
	}
	return model.HistoryPage{
		Items:    items,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *Store) Get(id string, page, pageSize int) (model.HistoryResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 100
	}
	if pageSize > 500 {
		pageSize = 500
	}

	var (
		run           model.RunRecord
		startedAt     sql.NullString
		endedAt       sql.NullString
		configJSON    string
		summaryJSON   sql.NullString
		exitCode      sql.NullInt64
		rawOutput     sql.NullString
		rawTruncated  sql.NullInt64
		stderrSummary sql.NullString
		stderrTrunc   sql.NullInt64
	)
	err := s.db.QueryRow(
		`SELECT id, created_at, started_at, ended_at, status, protocol, direction,
			host, port, config_json, output_mode, parser_version, exit_code,
			summary_json, stdout_text, stdout_truncated, stderr_summary, stderr_truncated
		FROM runs WHERE id = ?`, id,
	).Scan(
		&run.ID, &run.CreatedAt, &startedAt, &endedAt, &run.Status,
		&run.Protocol, &run.Direction, &run.Host, &run.Port, &configJSON,
		&run.OutputMode, &run.ParserVersion, &exitCode, &summaryJSON,
		&rawOutput, &rawTruncated, &stderrSummary, &stderrTrunc,
	)
	if err == sql.ErrNoRows {
		return model.HistoryResult{Error: &model.AppError{
			Code:    model.ErrHistoryNotFound,
			Stage:   "persist",
			Message: "历史记录不存在或已被删除",
		}}, nil
	}
	if err != nil {
		return model.HistoryResult{}, err
	}
	run.StartedAt = startedAt.String
	run.EndedAt = endedAt.String
	if exitCode.Valid {
		value := int(exitCode.Int64)
		run.ExitCode = &value
	}
	if configJSON != "" {
		_ = json.Unmarshal([]byte(configJSON), &run.Config)
	}
	if summaryJSON.Valid && summaryJSON.String != "" {
		var summary model.Summary
		if json.Unmarshal([]byte(summaryJSON.String), &summary) == nil {
			run.Summary = &summary
		}
	}

	var intervalTotal int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM intervals WHERE run_id = ?", id).Scan(&intervalTotal); err != nil {
		return model.HistoryResult{}, err
	}
	rows, err := s.db.Query(
		`SELECT run_id, sequence, direction, start_seconds, end_seconds,
			bits_per_second, bytes, tcp_retransmits,
			udp_jitter_ms, udp_lost, udp_total
		FROM intervals WHERE run_id = ?
		ORDER BY sequence ASC, direction ASC LIMIT ? OFFSET ?`,
		id, pageSize, (page-1)*pageSize,
	)
	if err != nil {
		return model.HistoryResult{}, err
	}
	defer rows.Close()
	intervals := make([]model.Interval, 0, pageSize)
	for rows.Next() {
		var item model.Interval
		if err := rows.Scan(
			&item.RunID, &item.Sequence, &item.Direction, &item.StartSeconds, &item.EndSeconds,
			&item.BitsPerSecond, &item.Bytes, &item.TCPRetransmits,
			&item.UDPJitterMilliseconds, &item.UDPLostDatagrams, &item.UDPTotalDatagrams,
		); err != nil {
			return model.HistoryResult{}, err
		}
		intervals = append(intervals, item)
	}
	if err := rows.Err(); err != nil {
		return model.HistoryResult{}, err
	}

	return model.HistoryResult{
		Run:             run,
		Intervals:       intervals,
		IntervalTotal:   intervalTotal,
		Page:            page,
		PageSize:        pageSize,
		RawOutput:       rawOutput.String,
		RawTruncated:    rawTruncated.Int64 != 0,
		StderrSummary:   stderrSummary.String,
		StderrTruncated: stderrTrunc.Int64 != 0,
	}, nil
}

func (s *Store) AllIntervals(runID string) ([]model.Interval, error) {
	rows, err := s.db.Query(
		`SELECT run_id, sequence, direction, start_seconds, end_seconds,
			bits_per_second, bytes, tcp_retransmits,
			udp_jitter_ms, udp_lost, udp_total
		FROM intervals WHERE run_id = ?
		ORDER BY sequence ASC, direction ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	intervals := make([]model.Interval, 0)
	for rows.Next() {
		var item model.Interval
		if err := rows.Scan(
			&item.RunID, &item.Sequence, &item.Direction, &item.StartSeconds, &item.EndSeconds,
			&item.BitsPerSecond, &item.Bytes, &item.TCPRetransmits,
			&item.UDPJitterMilliseconds, &item.UDPLostDatagrams, &item.UDPTotalDatagrams,
		); err != nil {
			return nil, err
		}
		intervals = append(intervals, item)
	}
	return intervals, rows.Err()
}

func (s *Store) Delete(ids []string) (int, error) {
	ids = compactIDs(ids)
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM intervals WHERE run_id IN ("+placeholders+")", args...); err != nil {
		return 0, err
	}
	result, err := tx.Exec("DELETE FROM runs WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (s *Store) Clear() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM intervals"); err != nil {
		return 0, err
	}
	result, err := tx.Exec("DELETE FROM runs")
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	_, _ = s.db.Exec("VACUUM")
	return int(affected), nil
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func nullableText(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func nullableSummary(summary *model.Summary) interface{} {
	if summary == nil {
		return nil
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return nil
	}
	return string(data)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func summaryBPS(summaryJSON string) (float64, float64) {
	if summaryJSON == "" {
		return 0, 0
	}
	var summary model.Summary
	if err := json.Unmarshal([]byte(summaryJSON), &summary); err != nil {
		return 0, 0
	}
	var upload, download float64
	if summary.Upload != nil {
		upload = summary.Upload.BitsPerSecond
	}
	if summary.Download != nil {
		download = summary.Download.BitsPerSecond
	}
	return upload, download
}

func compactIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}
