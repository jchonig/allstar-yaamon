package backup

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"time"

	"allstar-yaamon/internal/db"
)

// ExportINI serialises favorites to the AllScan/Supermon favorites.ini format.
func ExportINI(favs []db.Favorite, homeNodeNum string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "; YAAMon favorites export\n")
	fmt.Fprintf(&buf, "; Home node: %s\n", homeNodeNum)
	fmt.Fprintf(&buf, "; Exported: %s\n", time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))
	buf.WriteByte('\n')

	for _, f := range favs {
		fmt.Fprintf(&buf, "[node_%s]\n", f.NodeNumber)
		fmt.Fprintf(&buf, "cmd[] = \"rpt cmd %s ilink 3 %s\"\n\n", homeNodeNum, f.NodeNumber)
	}
	return buf.Bytes()
}

// ImportedFav is a favorite parsed from a favorites.ini file.
type ImportedFav struct {
	NodeNumber  string
	Callsign    string
	Description string
}

// ParseINI parses a favorites.ini file and returns the favorites it references.
// Tolerates AllScan, Supermon, and YAAMon-exported formats.
// AllScan label[] values are parsed to extract callsign and description.
func ParseINI(data []byte) ([]ImportedFav, error) {
	var favs []ImportedFav
	seen := make(map[string]bool)
	var pendingLabel string

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header: [node_12345] or [12345] or [general]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inner := line[1 : len(line)-1]
			num := strings.TrimPrefix(inner, "node_")
			num = strings.TrimSpace(num)
			if isNodeNumber(num) && !seen[num] {
				favs = append(favs, ImportedFav{NodeNumber: num})
				seen[num] = true
			}
			pendingLabel = ""
			continue
		}

		// label[] = "W1AW ARRL HQ, Newington CT"
		if strings.HasPrefix(line, "label[]") {
			if eq := strings.Index(line, "="); eq >= 0 {
				val := strings.TrimSpace(line[eq+1:])
				pendingLabel = strings.Trim(val, `"`)
			}
			continue
		}

		// cmd[] = "rpt cmd %node% ilink 3 67890" — extract target (last token)
		if strings.HasPrefix(line, "cmd[]") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				last := strings.Trim(parts[len(parts)-1], `"`)
				if isNodeNumber(last) && !seen[last] {
					cs, desc := parseLabel(pendingLabel)
					favs = append(favs, ImportedFav{NodeNumber: last, Callsign: cs, Description: desc})
					seen[last] = true
				}
			}
			pendingLabel = ""
		}
	}
	return favs, scanner.Err()
}

// parseLabel splits an AllScan label into callsign and description.
// The first word is treated as a callsign if it is 3–7 alphanumeric characters
// and contains at least one digit — matching the pattern of amateur radio callsigns.
func parseLabel(label string) (callsign, description string) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", ""
	}
	parts := strings.SplitN(label, " ", 2)
	first := parts[0]
	if len(first) >= 3 && len(first) <= 7 && isAlphanumeric(first) && containsDigit(first) {
		callsign = strings.ToUpper(first)
		if len(parts) > 1 {
			description = strings.TrimSpace(parts[1])
		}
		return
	}
	return "", label
}

func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func containsDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

func isNodeNumber(s string) bool {
	if len(s) == 0 || len(s) > 10 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
