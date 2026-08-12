package storage

import (
	"context"
	"strings"
	"testing"
)

func TestStorePersistsConversationsAndMessages(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	conversation, err := store.CreateConversation(context.Background(), "  选课助手  ")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.Title != "选课助手" {
		t.Fatalf("title = %q", conversation.Title)
	}
	if _, err := store.AppendMessage(context.Background(), conversation.ID, messageRoleUser, "帮我看看这学期能选什么课"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendMessage(context.Background(), conversation.ID, messageRoleAssistant, "我先整理课程和时间冲突。"); err != nil {
		t.Fatal(err)
	}

	messages, err := store.ListMessages(context.Background(), conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != messageRoleUser || messages[1].Role != messageRoleAssistant {
		t.Fatalf("unexpected messages: %#v", messages)
	}

	conversations, err := store.ListConversations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 || conversations[0].ID != conversation.ID {
		t.Fatalf("unexpected conversations: %#v", conversations)
	}
}

func TestStoreRejectsInvalidMessageAndMissingConversation(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if _, err := store.AppendMessage(context.Background(), "missing", messageRoleUser, "hello"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("unexpected missing conversation error: %v", err)
	}
	conversation, err := store.CreateConversation(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if conversation.Title != "新对话" {
		t.Fatalf("default title = %q", conversation.Title)
	}
	if _, err := store.AppendMessage(context.Background(), conversation.ID, "system", "hello"); err == nil {
		t.Fatal("unsupported role should fail")
	}
}

func TestStorePersistsToolAuditWithoutToolResultPayload(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	conversation, err := store.CreateConversation(context.Background(), "审计")
	if err != nil {
		t.Fatal(err)
	}
	started, err := store.AppendToolAudit(context.Background(), conversation.ID, "hdu_academic_schedule", ToolAuditStarted, "")
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != ToolAuditStarted || started.Detail != "" {
		t.Fatalf("unexpected started audit: %#v", started)
	}
	_, err = store.AppendToolAudit(context.Background(), conversation.ID, "hdu_academic_schedule", ToolAuditFailed, strings.Repeat("x", auditDetailLimit+20))
	if err != nil {
		t.Fatal(err)
	}
	audits, err := store.ListToolAudits(context.Background(), conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || len([]rune(audits[1].Detail)) != auditDetailLimit {
		t.Fatalf("unexpected audits: %#v", audits)
	}
}
