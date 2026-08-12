package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	conversationTitleLimit = 120
	messageRoleUser        = "user"
	messageRoleAssistant   = "assistant"
	ToolAuditStarted       = "started"
	ToolAuditCompleted     = "completed"
	ToolAuditFailed        = "failed"
	auditToolNameLimit     = 120
	auditDetailLimit       = 1000
)

type Store struct {
	db   *sql.DB
	path string
}

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}

type ToolAudit struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	ToolName       string    `json:"toolName"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

func Open(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("storage root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	path := filepath.Join(root, "station.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	store := &Store{db: db, path: path}
	db.SetMaxOpenConns(1)
	if err := store.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("secure SQLite database: %w", err)
	}
	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) initialize() error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
			content TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS messages_conversation_created_at
			ON messages (conversation_id, created_at, id)`,
		`CREATE TABLE IF NOT EXISTS tool_audits (
			id TEXT PRIMARY KEY,
			conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
			tool_name TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('started', 'completed', 'failed')),
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS tool_audits_conversation_created_at
			ON tool_audits (conversation_id, created_at, id)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("initialize SQLite database: %w", err)
		}
	}
	return nil
}

func (s *Store) ListConversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, created_at, updated_at
		FROM conversations
		ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]Conversation, 0)
	for rows.Next() {
		conversation, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, conversation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return conversations, nil
}

func (s *Store) CreateConversation(ctx context.Context, title string) (Conversation, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新对话"
	}
	if len([]rune(title)) > conversationTitleLimit {
		title = string([]rune(title)[:conversationTitleLimit])
	}

	now := time.Now().UTC()
	conversation := Conversation{
		ID:        newID("conversation"),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO conversations (id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)`, conversation.ID, conversation.Title, nowText(now), nowText(now)); err != nil {
		return Conversation{}, fmt.Errorf("create conversation: %w", err)
	}
	return conversation, nil
}

func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]Message, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("conversation id is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, role, content, created_at
		FROM messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC, id ASC`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return messages, nil
}

func (s *Store) AppendMessage(ctx context.Context, conversationID, role, content string) (Message, error) {
	conversationID = strings.TrimSpace(conversationID)
	content = strings.TrimSpace(content)
	if conversationID == "" {
		return Message{}, errors.New("conversation id is required")
	}
	if content == "" {
		return Message{}, errors.New("message content is required")
	}
	if role != messageRoleUser && role != messageRoleAssistant {
		return Message{}, fmt.Errorf("message role %q is not supported", role)
	}

	now := time.Now().UTC()
	message := Message{
		ID:             newID("message"),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		CreatedAt:      now,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Message{}, fmt.Errorf("begin append message: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `
		UPDATE conversations SET updated_at = ? WHERE id = ?`, nowText(now), conversationID)
	if err != nil {
		return Message{}, fmt.Errorf("touch conversation: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return Message{}, fmt.Errorf("check conversation: %w", err)
	} else if affected != 1 {
		return Message{}, fmt.Errorf("conversation %q does not exist", conversationID)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, role, content, created_at)
		VALUES (?, ?, ?, ?, ?)`, message.ID, message.ConversationID, message.Role, message.Content, nowText(now)); err != nil {
		return Message{}, fmt.Errorf("append message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Message{}, fmt.Errorf("commit message: %w", err)
	}
	return message, nil
}

func (s *Store) AppendToolAudit(ctx context.Context, conversationID, toolName, status, detail string) (ToolAudit, error) {
	conversationID = strings.TrimSpace(conversationID)
	toolName = strings.TrimSpace(toolName)
	detail = strings.TrimSpace(detail)
	if conversationID == "" {
		return ToolAudit{}, errors.New("conversation id is required")
	}
	if toolName == "" || len([]rune(toolName)) > auditToolNameLimit {
		return ToolAudit{}, errors.New("tool name is required and must be at most 120 characters")
	}
	switch status {
	case ToolAuditStarted, ToolAuditCompleted, ToolAuditFailed:
	default:
		return ToolAudit{}, fmt.Errorf("tool audit status %q is not supported", status)
	}
	if len([]rune(detail)) > auditDetailLimit {
		detail = string([]rune(detail)[:auditDetailLimit])
	}
	now := time.Now().UTC()
	audit := ToolAudit{
		ID:             newID("tool-audit"),
		ConversationID: conversationID,
		ToolName:       toolName,
		Status:         status,
		Detail:         detail,
		CreatedAt:      now,
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_audits (id, conversation_id, tool_name, status, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, audit.ID, audit.ConversationID, audit.ToolName, audit.Status, audit.Detail, nowText(now)); err != nil {
		return ToolAudit{}, fmt.Errorf("append tool audit: %w", err)
	}
	return audit, nil
}

func (s *Store) ListToolAudits(ctx context.Context, conversationID string) ([]ToolAudit, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, errors.New("conversation id is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, conversation_id, tool_name, status, detail, created_at
		FROM tool_audits
		WHERE conversation_id = ?
		ORDER BY created_at ASC, id ASC`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list tool audits: %w", err)
	}
	defer rows.Close()
	audits := make([]ToolAudit, 0)
	for rows.Next() {
		var audit ToolAudit
		var createdAt string
		if err := rows.Scan(&audit.ID, &audit.ConversationID, &audit.ToolName, &audit.Status, &audit.Detail, &createdAt); err != nil {
			return nil, fmt.Errorf("scan tool audit: %w", err)
		}
		audit.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse tool audit timestamp: %w", err)
		}
		audits = append(audits, audit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tool audits: %w", err)
	}
	return audits, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanConversation(row scanner) (Conversation, error) {
	var conversation Conversation
	var createdAt, updatedAt string
	if err := row.Scan(&conversation.ID, &conversation.Title, &createdAt, &updatedAt); err != nil {
		return Conversation{}, err
	}
	var err error
	conversation.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Conversation{}, err
	}
	conversation.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return Conversation{}, err
	}
	return conversation, nil
}

func scanMessage(row scanner) (Message, error) {
	var message Message
	var createdAt string
	if err := row.Scan(&message.ID, &message.ConversationID, &message.Role, &message.Content, &createdAt); err != nil {
		return Message{}, err
	}
	var err error
	message.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Message{}, err
	}
	return message, nil
}

func nowText(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", value, err)
	}
	return parsed, nil
}

func newID(prefix string) string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(bytes)
}
