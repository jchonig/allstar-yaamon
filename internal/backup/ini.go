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

// ImportedFav is a node number parsed from a favorites.ini file.
type ImportedFav struct {
	NodeNumber string
}

// ParseINI parses a favorites.ini file and returns the node numbers it references.
// Tolerates AllScan, Supermon, and YAAMon-exported formats.
func ParseINI(data []byte) ([]ImportedFav, error) {
	var favs []ImportedFav
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section header: [node_12345] or [12345]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inner := line[1 : len(line)-1]
			num := strings.TrimPrefix(inner, "node_")
			num = strings.TrimSpace(num)
			if isNodeNumber(num) && !seen[num] {
				favs = append(favs, ImportedFav{NodeNumber: num})
				seen[num] = true
			}
			continue
		}

		// cmd[] = "rpt cmd 12345 ilink 3 67890" — extract the target (last token)
		if strings.HasPrefix(line, "cmd[]") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				last := strings.Trim(parts[len(parts)-1], `"`)
				if isNodeNumber(last) && !seen[last] {
					favs = append(favs, ImportedFav{NodeNumber: last})
					seen[last] = true
				}
			}
		}
	}
	return favs, scanner.Err()
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
