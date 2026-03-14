package cliutil

import "strings"

type MultiValue struct {
	values []string
	set    bool
}

func (f *MultiValue) String() string {
	return strings.Join(f.values, ",")
}

func (f *MultiValue) Set(value string) error {
	f.set = true
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			f.values = append(f.values, part)
		}
	}
	return nil
}

func (f *MultiValue) Values(defaults ...string) []string {
	if f.set {
		return append([]string(nil), f.values...)
	}
	return append([]string(nil), defaults...)
}

func (f *MultiValue) First(defaultValue string) string {
	values := f.Values(defaultValue)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
