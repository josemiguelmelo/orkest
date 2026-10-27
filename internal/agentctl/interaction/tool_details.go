package interaction

import (
	"encoding/json"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxToolFieldBytes  = 16 * 1024
	MaxToolDetailBytes = 64 * 1024
)

// BoundToolUpdate limits persisted tool payloads, including metadata overhead.
func BoundToolUpdate(u Update) Update {
	truncate := func(s string, n int) (string, bool) {
		if len(s) <= n {
			return s, false
		}
		s = s[:n]
		for len(s) > 0 && !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
		return s, true
	}
	var cut bool
	u.ToolKind, cut = truncate(u.ToolKind, 128)
	u.Truncated = u.Truncated || cut
	u.Title, cut = truncate(u.Title, MaxToolFieldBytes)
	u.Truncated = u.Truncated || cut
	u.Input, cut = truncate(u.Input, MaxToolFieldBytes)
	u.Truncated = u.Truncated || cut
	u.Output, cut = truncate(u.Output, MaxToolFieldBytes)
	u.Truncated = u.Truncated || cut
	if len(u.Locations) > 32 {
		u.Locations = u.Locations[:32]
		u.Truncated = true
	}
	if len(u.Content) > 8 {
		u.Content = u.Content[:8]
		u.Truncated = true
	}
	for i := range u.Content {
		u.Content[i].Kind, cut = truncate(u.Content[i].Kind, 128)
		u.Truncated = u.Truncated || cut
		u.Content[i].Path, cut = truncate(u.Content[i].Path, MaxToolFieldBytes)
		u.Truncated = u.Truncated || cut
		u.Content[i].Text, cut = truncate(u.Content[i].Text, MaxToolFieldBytes)
		u.Truncated = u.Truncated || cut
		u.Content[i].OldText, cut = truncate(u.Content[i].OldText, MaxToolFieldBytes)
		u.Truncated = u.Truncated || cut
		u.Content[i].NewText, cut = truncate(u.Content[i].NewText, MaxToolFieldBytes)
		u.Truncated = u.Truncated || cut
		u.Content[i].Truncated = u.Content[i].Truncated || u.Truncated
	}
	for i := range u.Locations {
		u.Locations[i].Path, cut = truncate(u.Locations[i].Path, MaxToolFieldBytes)
		u.Truncated = u.Truncated || cut
	}
	for {
		b, _ := json.Marshal(u)
		if len(b) <= MaxToolDetailBytes {
			break
		}
		u.Truncated = true
		if len(u.Locations) > 0 {
			u.Locations = u.Locations[:len(u.Locations)-1]
		} else if u.Output != "" {
			u.Output, _ = truncate(u.Output, len(u.Output)*3/4)
		} else if len(u.Content) > 0 {
			i := len(u.Content) - 1
			u.Content[i].Truncated = true
			u.Content[i].NewText, cut = truncate(u.Content[i].NewText, len(u.Content[i].NewText)/2)
			if !cut {
				u.Content = u.Content[:i]
			}
		} else if u.Input != "" {
			u.Input, _ = truncate(u.Input, len(u.Input)*3/4)
		} else if u.Title != "" {
			u.Title, _ = truncate(u.Title, len(u.Title)/2)
		} else {
			break
		}
	}
	return u
}

// RedactToolUpdate removes common credential fields and known secret values before persistence.
func RedactToolUpdate(u Update, secrets []string) Update {
	u.Locations = slices.Clone(u.Locations)
	u.Content = slices.Clone(u.Content)
	clean := func(s string) string {
		for _, secret := range secrets {
			if secret != "" {
				s = strings.ReplaceAll(s, secret, "[redacted]")
			}
		}
		return s
	}
	u.Title = clean(u.Title)
	u.Input = redactJSON(u.Input, clean)
	u.Output = redactJSON(u.Output, clean)
	for i := range u.Locations {
		u.Locations[i].Path = clean(u.Locations[i].Path)
	}
	for i := range u.Content {
		c := &u.Content[i]
		c.Path = clean(c.Path)
		c.Text = redactJSON(c.Text, clean)
		c.OldText = clean(c.OldText)
		c.NewText = clean(c.NewText)
	}
	return u
}

func redactJSON(s string, clean func(string) string) string {
	var value any
	decoder := json.NewDecoder(strings.NewReader(s))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return clean(s)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return clean(s)
	}
	value = redactValue(value, clean)
	b, err := json.Marshal(value)
	if err != nil {
		return "[unavailable]"
	}
	return string(b)
}

func redactValue(v any, clean func(string) string) any {
	switch x := v.(type) {
	case string:
		return clean(x)
	case map[string]any:
		for k, val := range x {
			low := strings.ToLower(k)
			if strings.Contains(low, "password") || strings.Contains(low, "secret") || strings.Contains(low, "token") || strings.Contains(low, "authorization") || strings.Contains(low, "api_key") || strings.Contains(low, "apikey") {
				x[k] = "[redacted]"
			} else {
				x[k] = redactValue(val, clean)
			}
		}
	case []any:
		for i, val := range x {
			x[i] = redactValue(val, clean)
		}
	}
	return v
}
