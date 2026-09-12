package storage

import (
	"context"
	"database/sql"
	"fmt"
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

func TestObsoleteCourseCacheIsRemovedWithoutLosingChats(t *testing.T) {
	for _, priorVersion := range []int{1, 2} {
		t.Run(fmt.Sprint(priorVersion), func(t *testing.T) {
			root, ctx := t.TempDir(), context.Background()
			s, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='course_type_cache'").Scan(&count); err != nil || count != 0 {
				t.Fatal("new database created the retired course cache")
			}
			conversation, _, answer, err := s.Begin(ctx, "", "已有的对话")
			if err != nil {
				t.Fatal(err)
			}
			answer.Content, answer.State = "需要保留的回答", "complete"
			if err := s.Finish(ctx, answer); err != nil {
				t.Fatal(err)
			}
			if priorVersion == 2 {
				_, err = s.db.Exec(`CREATE TABLE course_type_cache(cache_key TEXT PRIMARY KEY,payload BLOB NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL);
 CREATE INDEX course_type_cache_expiry ON course_type_cache(expires_at);
 INSERT INTO course_type_cache VALUES ('old', '{}', 0, 1);`)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version=%d", priorVersion)); err != nil {
				t.Fatal(err)
			}
			s.Close()
			for i := 0; i < 2; i++ {
				s, err = Open(root)
				if err != nil {
					t.Fatal(err)
				}
				messages, err := s.Messages(ctx, conversation.ID)
				if err != nil || len(messages) != 2 || messages[1].Content != answer.Content || messages[1].State != "complete" {
					t.Fatal("course cache removal changed visible conversation history")
				}
				var version int
				if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 3 {
					t.Fatal("database did not advance to version 3")
				}
				if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name IN ('course_type_cache','course_type_cache_expiry')").Scan(&count); err != nil || count != 0 {
					t.Fatal("obsolete course cache survived migration")
				}
				s.Close()
			}
		})
	}
}
