package database

import "strings"

// SplitStatements splits a SQL script on top-level semicolons. It understands
// quoted strings and identifiers, line and block comments, and PostgreSQL
// dollar-quoted bodies, so functions and string literals stay intact.
// Statements that contain only whitespace or comments are dropped.
func SplitStatements(script, engine string) []string {
	statements := []string{}
	var current strings.Builder
	meaningful := false
	flush := func() {
		text := strings.TrimSpace(current.String())
		if meaningful && text != "" {
			statements = append(statements, text)
		}
		current.Reset()
		meaningful = false
	}
	mysql := engine == "mysql"
	n := len(script)
	for i := 0; i < n; {
		c := script[i]
		switch {
		case c == ';':
			flush()
			i++
			continue
		case c == '-' && i+1 < n && script[i+1] == '-', mysql && c == '#':
			end := strings.IndexByte(script[i:], '\n')
			if end < 0 {
				end = n - i
			}
			current.WriteString(script[i : i+end])
			i += end
			continue
		case c == '/' && i+1 < n && script[i+1] == '*':
			end := strings.Index(script[i+2:], "*/")
			if end < 0 {
				end = n - i - 2
			} else {
				end += 2
			}
			current.WriteString(script[i : i+2+end])
			i += 2 + end
			continue
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for j < n {
				if mysql && script[j] == '\\' && c != '`' {
					j += 2
					continue
				}
				if script[j] == c {
					if j+1 < n && script[j+1] == c {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= n {
				j = n - 1
			}
			current.WriteString(script[i : j+1])
			meaningful = true
			i = j + 1
			continue
		case c == '$' && !mysql:
			if tag, ok := dollarTag(script[i:]); ok {
				end := strings.Index(script[i+len(tag):], tag)
				stop := n
				if end >= 0 {
					stop = i + len(tag) + end + len(tag)
				}
				current.WriteString(script[i:stop])
				meaningful = true
				i = stop
				continue
			}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			meaningful = true
		}
		current.WriteByte(c)
		i++
	}
	flush()
	return statements
}

// dollarTag returns "$tag$" when text starts with a PostgreSQL dollar quote.
func dollarTag(text string) (string, bool) {
	for j := 1; j < len(text); j++ {
		c := text[j]
		if c == '$' {
			return text[:j+1], true
		}
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
		digit := c >= '0' && c <= '9'
		if !letter && (!digit || j == 1) {
			return "", false
		}
	}
	return "", false
}
