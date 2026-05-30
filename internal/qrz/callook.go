package qrz

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SetCredentials updates QRZ credentials without discarding the in-memory cache.
func (c *Client) SetCredentials(username, password string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.username = username
	c.password = password
	c.sessionKey = "" // invalidate any cached session
}

// LookupCallook fetches a callsign record from callook.info (US FCC data, no auth required).
// Results share the same in-memory cache as QRZ lookups.
func (c *Client) LookupCallook(ctx context.Context, callsign string, saver Saver) (Record, error) {
	c.mu.Lock()
	if rec, ok := c.cache[callsign]; ok && !rec.Stale() {
		c.mu.Unlock()
		return rec, nil
	}
	c.mu.Unlock()

	rec, err := fetchCallook(ctx, callsign)
	if err != nil {
		return Record{}, err
	}

	c.mu.Lock()
	c.cache[callsign] = rec
	c.mu.Unlock()

	if saver != nil {
		raw, merr := json.Marshal(rec)
		if merr == nil {
			if serr := saver.SaveQRZRecord(ctx, callsign, json.RawMessage(raw), rec.FetchedAt); serr != nil {
				// Non-fatal; log via caller if needed.
				_ = serr
			}
		}
	}
	return rec, nil
}

type callookResponse struct {
	Status  string `json:"status"`
	Current struct {
		Callsign  string `json:"callsign"`
		OperClass string `json:"operClass"`
	} `json:"current"`
	Name    string `json:"name"`
	Address struct {
		Line1   string `json:"line1"`
		Line2   string `json:"line2"`
		Country string `json:"country"`
	} `json:"address"`
}

func fetchCallook(ctx context.Context, callsign string) (Record, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://callook.info/"+strings.ToUpper(callsign)+"/json", nil)
	if err != nil {
		return Record{}, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Record{}, err
	}
	defer res.Body.Close()

	var r callookResponse
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return Record{}, fmt.Errorf("callook parse: %w", err)
	}
	if r.Status != "VALID" {
		return Record{}, fmt.Errorf("callook: %s not found", callsign)
	}

	// callook returns names in ALL-CAPS; title-case them.
	name := titleCase(r.Name)
	first, last := splitName(name)

	// line2 format: "CITY, ST  ZIP" — extract the 2-letter state code.
	state := parseState(r.Address.Line2)
	country := r.Address.Country
	if country == "" {
		country = "United States"
	}

	return Record{
		Callsign:  strings.ToUpper(callsign),
		FirstName: first,
		LastName:  last,
		Address:   titleCase(r.Address.Line2),
		State:     state,
		Country:   country,
		Class:     r.Current.OperClass,
		FetchedAt: time.Now().UTC(),
	}, nil
}

func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func splitName(name string) (first, last string) {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return "", parts[0]
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// parseState extracts the 2-letter state code from callook address line2.
// Format: "NEWINGTON, CT  06111"
func parseState(line2 string) string {
	parts := strings.SplitN(line2, ",", 2)
	if len(parts) < 2 {
		return ""
	}
	fields := strings.Fields(parts[1])
	if len(fields) == 0 {
		return ""
	}
	st := strings.ToUpper(fields[0])
	if len(st) == 2 {
		return st
	}
	return ""
}
