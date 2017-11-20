package utils

import (
	"net/textproto"
)

type Headers map[string]bool

func (h Headers) Add(header string) {
	canonicalHeader := textproto.CanonicalMIMEHeaderKey(header)
	h[canonicalHeader] = true
}

func (h Headers) Contains(header string) bool {
	canonicalHeader := textproto.CanonicalMIMEHeaderKey(header)
	_, isPresent := h[canonicalHeader]
	return isPresent
}

func (h Headers) Strings() []string {
	headers := []string{}
	for header, _ := range h {
		headers = append(headers, header)
	}
	return headers
}
