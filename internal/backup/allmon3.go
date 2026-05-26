package backup

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// AllmonNode is a node parsed from an Allmon3 allmon3.ini file.
type AllmonNode struct {
	NodeNumber string `json:"node_number"`
	AMIHost    string `json:"ami_host"`
	AMIPort    int    `json:"ami_port"`
	AMIUser    string `json:"ami_user"`
	AMIPass    string `json:"ami_pass"`
}

// ParseAllmon3INI parses an Allmon3 allmon3.ini file and returns the nodes it defines.
// Each section whose name is a node number becomes an AllmonNode.
// Unrecognised sections (e.g. [general]) are silently ignored.
func ParseAllmon3INI(data []byte) ([]AllmonNode, error) {
	var nodes []AllmonNode
	var cur *AllmonNode

	flush := func() {
		if cur != nil {
			nodes = append(nodes, *cur)
			cur = nil
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			inner := strings.TrimSpace(line[1 : len(line)-1])
			if isNodeNumber(inner) {
				cur = &AllmonNode{
					NodeNumber: inner,
					AMIHost:    "localhost",
					AMIPort:    5038,
				}
			}
			continue
		}

		if cur == nil {
			continue
		}

		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])

		switch key {
		case "host":
			cur.AMIHost = val
		case "port":
			if p, err := strconv.Atoi(val); err == nil {
				cur.AMIPort = p
			}
		case "user":
			cur.AMIUser = val
		case "pass":
			cur.AMIPass = val
		}
	}
	flush()
	return nodes, scanner.Err()
}
