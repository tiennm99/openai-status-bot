package bot

import "testing"

func TestMenuCommandsMatchSupportedCommands(t *testing.T) {
	commands := MenuCommands()
	got := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.Command == "" {
			t.Fatal("command is empty")
		}
		if command.Command[0] == '/' {
			t.Fatalf("command %q must not include leading slash", command.Command)
		}
		if command.Description == "" {
			t.Fatalf("description for %q is empty", command.Command)
		}
		got = append(got, command.Command)
	}

	want := []string{"start", "stop", "status", "components", "subscribe", "history", "uptime", "info", "help"}
	if len(got) != len(want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commands = %v, want %v", got, want)
		}
	}
}
