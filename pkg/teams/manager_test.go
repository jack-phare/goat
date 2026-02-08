package teams

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jg-phare/goat/pkg/agent"
)

func newTestManager(t *testing.T) *TeamManager {
	t.Helper()
	baseDir := t.TempDir()
	return NewTeamManager(&agent.NoOpHookRunner{}, baseDir)
}

func TestManagerCreateTeam(t *testing.T) {
	tm := newTestManager(t)

	team, err := tm.CreateTeam(context.Background(), "test-team")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}

	if team.Name != "test-team" {
		t.Errorf("expected test-team, got %s", team.Name)
	}

	// Verify config file exists
	configPath := filepath.Join(tm.baseDir, "teams", "test-team", "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}

	var config TeamConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if config.Name != "test-team" {
		t.Errorf("config name mismatch: %s", config.Name)
	}
}

func TestManagerCreateTeamCreatesDirectories(t *testing.T) {
	tm := newTestManager(t)
	tm.CreateTeam(context.Background(), "dir-test")

	// Tasks dir
	tasksDir := filepath.Join(tm.baseDir, "tasks", "dir-test")
	if _, err := os.Stat(tasksDir); err != nil {
		t.Errorf("tasks directory not created: %v", err)
	}

	// Mailbox dir
	mailboxDir := filepath.Join(tm.baseDir, "teams", "dir-test", "mailbox")
	if _, err := os.Stat(mailboxDir); err != nil {
		t.Errorf("mailbox directory not created: %v", err)
	}
}

func TestManagerOneTeamPerSession(t *testing.T) {
	tm := newTestManager(t)
	ctx := context.Background()

	_, err := tm.CreateTeam(ctx, "first-team")
	if err != nil {
		t.Fatalf("first CreateTeam: %v", err)
	}

	_, err = tm.CreateTeam(ctx, "second-team")
	if err == nil {
		t.Fatal("expected error creating second team")
	}
}

func TestManagerCreateTeamEmptyName(t *testing.T) {
	tm := newTestManager(t)
	_, err := tm.CreateTeam(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestManagerGetTeam(t *testing.T) {
	tm := newTestManager(t)

	// Before creation
	if team := tm.GetTeam(); team != nil {
		t.Fatal("expected nil before creation")
	}

	tm.CreateTeam(context.Background(), "get-test")

	team := tm.GetTeam()
	if team == nil {
		t.Fatal("expected non-nil after creation")
	}
	if team.Name != "get-test" {
		t.Errorf("expected get-test, got %s", team.Name)
	}
}

func TestManagerCleanup(t *testing.T) {
	tm := newTestManager(t)
	ctx := context.Background()

	tm.CreateTeam(ctx, "cleanup-test")

	if err := tm.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}

	// Team dir should be removed
	teamDir := filepath.Join(tm.baseDir, "teams", "cleanup-test")
	if _, err := os.Stat(teamDir); !os.IsNotExist(err) {
		t.Errorf("team directory still exists after cleanup")
	}

	// Tasks dir should be removed
	tasksDir := filepath.Join(tm.baseDir, "tasks", "cleanup-test")
	if _, err := os.Stat(tasksDir); !os.IsNotExist(err) {
		t.Errorf("tasks directory still exists after cleanup")
	}

	// Should be able to create a new team
	if team := tm.GetTeam(); team != nil {
		t.Fatal("expected nil after cleanup")
	}
}

func TestManagerCleanupNoTeam(t *testing.T) {
	tm := newTestManager(t)
	if err := tm.Cleanup(context.Background()); err == nil {
		t.Fatal("expected error cleaning up with no active team")
	}
}

func TestManagerCleanupWithActiveMembers(t *testing.T) {
	tm := newTestManager(t)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "active-members")
	team.Members["agent-1"] = &TeamMember{
		Name:  "agent-1",
		State: MemberActive,
	}

	if err := tm.Cleanup(ctx); err == nil {
		t.Fatal("expected error cleaning up with active members")
	}
}

func TestManagerCleanupWithStoppedMembers(t *testing.T) {
	tm := newTestManager(t)
	ctx := context.Background()

	team, _ := tm.CreateTeam(ctx, "stopped-members")
	team.Members["agent-1"] = &TeamMember{
		Name:  "agent-1",
		State: MemberStopped,
	}

	if err := tm.Cleanup(ctx); err != nil {
		t.Fatalf("Cleanup should succeed with stopped members: %v", err)
	}
}

func TestManagerCleanupThenRecreate(t *testing.T) {
	tm := newTestManager(t)
	ctx := context.Background()

	tm.CreateTeam(ctx, "first")
	tm.Cleanup(ctx)

	team, err := tm.CreateTeam(ctx, "second")
	if err != nil {
		t.Fatalf("CreateTeam after cleanup: %v", err)
	}
	if team.Name != "second" {
		t.Errorf("expected second, got %s", team.Name)
	}
}

func TestTeamAddMember(t *testing.T) {
	tm := newTestManager(t)
	team, _ := tm.CreateTeam(context.Background(), "member-test")

	member := &TeamMember{
		Name:      "worker",
		AgentID:   "agent-123",
		AgentType: "general-purpose",
		State:     MemberActive,
		PID:       12345,
	}

	if err := team.AddMember(member); err != nil {
		t.Fatalf("AddMember: %v", err)
	}

	// Verify config was persisted
	data, _ := os.ReadFile(team.configPath)
	var config TeamConfig
	json.Unmarshal(data, &config)

	if len(config.Members) != 1 {
		t.Fatalf("expected 1 member in config, got %d", len(config.Members))
	}
	if config.Members[0].Name != "worker" {
		t.Errorf("expected worker, got %s", config.Members[0].Name)
	}
	if config.Members[0].PID != 12345 {
		t.Errorf("expected PID 12345, got %d", config.Members[0].PID)
	}
}

func TestTeamGetMember(t *testing.T) {
	tm := newTestManager(t)
	team, _ := tm.CreateTeam(context.Background(), "get-member")

	team.Members["alice"] = &TeamMember{Name: "alice"}

	m, ok := team.GetMember("alice")
	if !ok || m.Name != "alice" {
		t.Error("expected to find alice")
	}

	_, ok = team.GetMember("bob")
	if ok {
		t.Error("did not expect to find bob")
	}
}

func TestTeamMemberNames(t *testing.T) {
	tm := newTestManager(t)
	team, _ := tm.CreateTeam(context.Background(), "names-test")

	team.Members["alice"] = &TeamMember{Name: "alice"}
	team.Members["bob"] = &TeamMember{Name: "bob"}

	names := team.MemberNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestMemberState(t *testing.T) {
	m := &TeamMember{State: MemberActive}
	if m.GetState() != MemberActive {
		t.Error("expected active")
	}

	m.SetState(MemberIdle)
	if m.GetState() != MemberIdle {
		t.Error("expected idle")
	}

	m.SetState(MemberStopped)
	if m.GetState() != MemberStopped {
		t.Error("expected stopped")
	}
}

func TestMemberStateString(t *testing.T) {
	tests := []struct {
		state MemberState
		want  string
	}{
		{MemberActive, "active"},
		{MemberIdle, "idle"},
		{MemberStopped, "stopped"},
		{MemberState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("MemberState(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}
