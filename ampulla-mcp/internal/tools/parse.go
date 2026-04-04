package tools

import (
	"encoding/json"
	"strings"
)

// Limits for truncation.
const (
	maxStackFrames  = 30
	maxBreadcrumbs  = 20
	maxTags         = 50
	maxStringBytes  = 1000
	maxHeaders      = 10
	maxHeaderValue  = 200
)

// Redacted header names (lowercase).
var redactedHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
}

// ParsedEvent holds structured fields extracted from a raw Ampulla event.
type ParsedEvent struct {
	Stacktrace  []StackFrame  `json:"stacktrace"`
	Tags        []Tag         `json:"tags"`
	User        *UserInfo     `json:"user,omitempty"`
	Request     *RequestInfo  `json:"request,omitempty"`
	Breadcrumbs []Breadcrumb  `json:"breadcrumbs"`
}

type StackFrame struct {
	Filename string `json:"filename,omitempty"`
	Function string `json:"function,omitempty"`
	Module   string `json:"module,omitempty"`
	Lineno   int    `json:"lineno,omitempty"`
	Colno    int    `json:"colno,omitempty"`
	AbsPath  string `json:"absPath,omitempty"`
	Context  string `json:"context,omitempty"`
}

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UserInfo struct {
	ID       string `json:"id,omitempty"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
	IPAddr   string `json:"ip_address,omitempty"`
}

type RequestInfo struct {
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type Breadcrumb struct {
	Timestamp string `json:"timestamp,omitempty"`
	Type      string `json:"type,omitempty"`
	Category  string `json:"category,omitempty"`
	Message   string `json:"message,omitempty"`
	Level     string `json:"level,omitempty"`
}

// ParseEventData extracts structured fields from a raw Ampulla event context blob.
func ParseEventData(raw json.RawMessage) ParsedEvent {
	var p ParsedEvent
	if len(raw) == 0 {
		return p
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(raw, &data); err != nil {
		return p
	}

	p.Stacktrace = extractStacktrace(data)
	p.Tags = extractTags(data)
	p.User = extractUser(data)
	p.Request = extractRequest(data)
	p.Breadcrumbs = extractBreadcrumbs(data)

	if p.Stacktrace == nil {
		p.Stacktrace = []StackFrame{}
	}
	if p.Tags == nil {
		p.Tags = []Tag{}
	}
	if p.Breadcrumbs == nil {
		p.Breadcrumbs = []Breadcrumb{}
	}

	return p
}

func extractStacktrace(data map[string]json.RawMessage) []StackFrame {
	// Try exception -> values[0] -> stacktrace -> frames
	if exc, ok := data["exception"]; ok {
		var excObj struct {
			Values []struct {
				Stacktrace struct {
					Frames []StackFrame `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		}
		if json.Unmarshal(exc, &excObj) == nil && len(excObj.Values) > 0 {
			frames := excObj.Values[0].Stacktrace.Frames
			return truncateFrames(frames)
		}
	}

	// Try top-level stacktrace -> frames
	if st, ok := data["stacktrace"]; ok {
		var stObj struct {
			Frames []StackFrame `json:"frames"`
		}
		if json.Unmarshal(st, &stObj) == nil {
			return truncateFrames(stObj.Frames)
		}
	}

	return nil
}

func truncateFrames(frames []StackFrame) []StackFrame {
	if len(frames) > maxStackFrames {
		frames = frames[len(frames)-maxStackFrames:]
	}
	for i := range frames {
		frames[i].Filename = truncStr(frames[i].Filename)
		frames[i].Function = truncStr(frames[i].Function)
		frames[i].Module = truncStr(frames[i].Module)
		frames[i].AbsPath = truncStr(frames[i].AbsPath)
		frames[i].Context = truncStr(frames[i].Context)
	}
	return frames
}

func extractTags(data map[string]json.RawMessage) []Tag {
	raw, ok := data["tags"]
	if !ok {
		return nil
	}

	// Tags can be [[key, value], ...] or {key: value}
	var arr [][]string
	if json.Unmarshal(raw, &arr) == nil {
		var tags []Tag
		for _, pair := range arr {
			if len(pair) == 2 {
				tags = append(tags, Tag{Key: truncStr(pair[0]), Value: truncStr(pair[1])})
			}
			if len(tags) >= maxTags {
				break
			}
		}
		return tags
	}

	var obj map[string]string
	if json.Unmarshal(raw, &obj) == nil {
		var tags []Tag
		for k, v := range obj {
			tags = append(tags, Tag{Key: truncStr(k), Value: truncStr(v)})
			if len(tags) >= maxTags {
				break
			}
		}
		return tags
	}

	return nil
}

func extractUser(data map[string]json.RawMessage) *UserInfo {
	raw, ok := data["user"]
	if !ok {
		return nil
	}
	var u UserInfo
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil
	}
	if u.ID == "" && u.Email == "" && u.Username == "" && u.IPAddr == "" {
		return nil
	}
	u.ID = truncStr(u.ID)
	u.Email = truncStr(u.Email)
	u.Username = truncStr(u.Username)
	u.IPAddr = truncStr(u.IPAddr)
	return &u
}

func extractRequest(data map[string]json.RawMessage) *RequestInfo {
	raw, ok := data["request"]
	if !ok {
		return nil
	}
	var r struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil
	}

	info := &RequestInfo{
		URL:    truncStr(r.URL),
		Method: r.Method,
	}

	if len(r.Headers) > 0 {
		info.Headers = make(map[string]string)
		count := 0
		for k, v := range r.Headers {
			if redactedHeaders[strings.ToLower(k)] {
				continue
			}
			if len(v) > maxHeaderValue {
				v = v[:maxHeaderValue]
			}
			info.Headers[k] = v
			count++
			if count >= maxHeaders {
				break
			}
		}
	}

	return info
}

func extractBreadcrumbs(data map[string]json.RawMessage) []Breadcrumb {
	raw, ok := data["breadcrumbs"]
	if !ok {
		return nil
	}

	// Breadcrumbs can be {values: [...]} or [...]
	var wrapper struct {
		Values []Breadcrumb `json:"values"`
	}
	if json.Unmarshal(raw, &wrapper) == nil && len(wrapper.Values) > 0 {
		return truncBreadcrumbs(wrapper.Values)
	}

	var arr []Breadcrumb
	if json.Unmarshal(raw, &arr) == nil {
		return truncBreadcrumbs(arr)
	}

	return nil
}

func truncBreadcrumbs(bcs []Breadcrumb) []Breadcrumb {
	if len(bcs) > maxBreadcrumbs {
		bcs = bcs[len(bcs)-maxBreadcrumbs:]
	}
	for i := range bcs {
		bcs[i].Message = truncStr(bcs[i].Message)
		bcs[i].Category = truncStr(bcs[i].Category)
	}
	return bcs
}

func truncStr(s string) string {
	if len(s) <= maxStringBytes {
		return s
	}
	return s[:maxStringBytes]
}
