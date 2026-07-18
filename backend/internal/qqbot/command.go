package qqbot

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"
)

type CommandKind string

const (
	CommandNone CommandKind = ""
	CommandHelp CommandKind = "help"
	CommandBind CommandKind = "bind"
)

type Command struct {
	Kind  CommandKind
	Email string
}

var mentionPattern = regexp.MustCompile(`<@!?[^>]+>`)

func ParseCommand(raw string) Command {
	cleaned := mentionPattern.ReplaceAllString(raw, " ")
	cleaned = strings.Map(func(r rune) rune {
		if r == '\u3000' || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, cleaned)
	fields := strings.Fields(cleaned)
	if len(fields) == 0 {
		return Command{}
	}
	switch strings.ToLower(strings.TrimSpace(fields[0])) {
	case "/help", "help", "帮助", "/帮助":
		return Command{Kind: CommandHelp}
	case "/bind", "bind", "绑定", "/绑定":
		if len(fields) != 2 {
			return Command{Kind: CommandBind}
		}
		return Command{Kind: CommandBind, Email: NormalizeEmail(fields[1])}
	default:
		return Command{}
	}
}

func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ValidEmail(value string) bool {
	value = NormalizeEmail(value)
	if value == "" || len(value) > 254 || strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return false
	}
	parts := strings.Split(value, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".")
}

func MaskEmail(value string) string {
	value = NormalizeEmail(value)
	parts := strings.Split(value, "@")
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := []rune(parts[0])
	if len(local) == 1 {
		return string(local[0]) + "***@" + parts[1]
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + parts[1]
}
