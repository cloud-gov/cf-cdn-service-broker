package utils

import (
	"net/textproto"
)

type Headers map[string]bool

func (h Headers) Add(header string) {
	canonical_header := textproto.CanonicalMIMEHeaderKey(header)
	h[canonical_header] = true
}

func (h Headers) Contains(header string) bool {
	canonical_header := textproto.CanonicalMIMEHeaderKey(header)
	_, is_present := h[canonical_header]
	return is_present
}

func (h Headers) Strings() []string {
	headers := []string{}
	for header, _ := range h {
		headers = append(headers, header)
	}
	return headers
}
