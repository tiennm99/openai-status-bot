package bot

import "strings"

func normalizeCommand(text, botUsername string) (string, []string) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", nil
	}
	command := strings.ToLower(fields[0])
	if at := strings.Index(command, "@"); at >= 0 {
		target := command[at+1:]
		if botUsername == "" || !strings.EqualFold(target, botUsername) {
			return "", fields
		}
		command = command[:at]
	}
	return command, fields
}
