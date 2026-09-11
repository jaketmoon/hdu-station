package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMessagesSurviveRestartAndInterruptedTurnIsMarked(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	c, u, a, err := s.Begin(ctx, "", "通识选修怎么选？")
	if err != nil {
		t.Fatal(err)
	}
	a.Content = "已经生成的部分"
	if err := s.Finish(ctx, a); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	messages, err := s.Messages(ctx, c.ID)
	if err != nil || len(messages) != 2 || messages[0].ID != u.ID || messages[1].State != "interrupted" || messages[1].Content != a.Content {
		t.Fatal("restart lost visible messages")
	}
	if err := s.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	messages, _ = s.Messages(ctx, c.ID)
	if len(messages) != 0 {
		t.Fatal("conversation deletion did not cascade")
	}
}
func TestInvalidConversationDoesNotLeaveHalfTurn(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, _, _, err = s.Begin(context.Background(), "missing", "hello")
	if err == nil {
		t.Fatal("missing conversation accepted")
	}
	items, _ := s.Conversations(context.Background())
	if len(items) != 0 {
		t.Fatal("failed transaction left a conversation")
	}
}
func TestFutureSchemaIsNotRewritten(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "station.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("PRAGMA user_version=99")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := Open(root); err == nil {
		s.Close()
		t.Fatal("future schema accepted")
	}
	db, _ = sql.Open("sqlite", filepath.Join(root, "station.db"))
	defer db.Close()
	var version int
	_ = db.QueryRow("PRAGMA user_version").Scan(&version)
	if version != 99 {
		t.Fatal("future database mutated")
	}
}
