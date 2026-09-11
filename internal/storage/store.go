package storage

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Conversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
}
type Message struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	State          string `json:"state"`
	CreatedAt      string `json:"createdAt"`
}
type Store struct{ db *sql.DB }

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(root, "station.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) { db.Close(); return nil, err }
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(errors.New("对话数据库版本较新，请升级应用"))
	}
	_, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
 CREATE TABLE IF NOT EXISTS conversations(id TEXT PRIMARY KEY,title TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS messages(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT UNIQUE NOT NULL,conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,role TEXT NOT NULL,content TEXT NOT NULL,state TEXT NOT NULL,created_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS message_conversation ON messages(conversation_id,seq);
 PRAGMA user_version=1;
 UPDATE messages SET state='interrupted' WHERE state='streaming';`)
	if err != nil {
		return fail(err)
	}
	return &Store{db: db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Conversations(ctx context.Context) ([]Conversation, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,title,updated_at FROM conversations ORDER BY updated_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
func (s *Store) Messages(ctx context.Context, id string) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,conversation_id,role,content,state,created_at FROM (SELECT * FROM messages WHERE conversation_id=? ORDER BY seq DESC LIMIT 200) ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Message{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.State, &m.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// Both visible messages are committed together, including the interrupted-turn marker.
func (s *Store) Begin(ctx context.Context, id, question string) (Conversation, Message, Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Conversation{}, Message{}, Message{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	c := Conversation{ID: id, UpdatedAt: now}
	if id == "" {
		c.ID = uuid.NewString()
		title := []rune(question)
		if len(title) > 26 {
			title = append(title[:26], '…')
		}
		c.Title = string(title)
		_, err = tx.ExecContext(ctx, "INSERT INTO conversations VALUES (?,?,?)", c.ID, c.Title, now)
	} else {
		err = tx.QueryRowContext(ctx, "SELECT title FROM conversations WHERE id=?", id).Scan(&c.Title)
		if err == nil {
			_, err = tx.ExecContext(ctx, "UPDATE conversations SET updated_at=? WHERE id=?", now, id)
		}
	}
	if err != nil {
		return c, Message{}, Message{}, err
	}
	u := Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "user", Content: question, State: "complete", CreatedAt: now}
	a := Message{ID: uuid.NewString(), ConversationID: c.ID, Role: "assistant", State: "streaming", CreatedAt: now}
	for _, m := range []Message{u, a} {
		if _, err = tx.ExecContext(ctx, "INSERT INTO messages(id,conversation_id,role,content,state,created_at) VALUES (?,?,?,?,?,?)", m.ID, m.ConversationID, m.Role, m.Content, m.State, m.CreatedAt); err != nil {
			return c, u, a, err
		}
	}
	return c, u, a, tx.Commit()
}
func (s *Store) Finish(ctx context.Context, m Message) error {
	_, err := s.db.ExecContext(ctx, "UPDATE messages SET content=?,state=? WHERE id=?", m.Content, m.State, m.ID)
	return err
}
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM conversations WHERE id=?", id)
	return err
}
