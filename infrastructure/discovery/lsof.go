package discovery

import (
	"bufio"
	"bytes"
	"slices"
	"strconv"
	"strings"
)

// parseLsof reads lsof -F pcn output.
func parseLsof(out []byte, gamePID int) []int {
	var first, rest []int
	pid, command := 0, ""
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'c':
			command = line[1:]
		case 'n':
			_, portText, ok := strings.Cut(line[1:], "127.0.0.1:")
			port, err := strconv.Atoi(portText)
			if !ok || err != nil || port < 1024 {
				continue
			}
			switch {
			case pid == gamePID:
				first = append(first, port)
			case strings.Contains(command, "Warcraft") || strings.Contains(command, "Blizzard"):
				rest = append(rest, port)
			}
		}
	}
	slices.Sort(first)
	slices.Sort(rest)
	return slices.Compact(append(first, rest...))
}
