package demo

import (
	"html"
	"strings"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
)

// renderHTML is the dataset's stand-in for the rendering Bitbucket serves next
// to every description and comment body — content.html and summary.html. It is
// deliberately not a Markdown renderer: the fixtures only need the one part of
// the rendering the reader depends on, which is how a mention comes back.
//
// An empty body renders to nothing at all, as it does on the wire for a deleted
// comment. Anything else is HTML-escaped and wrapped in a paragraph, with every
// @{account_id} that names one of accounts replaced by the mention span
// Bitbucket writes. An id belonging to nobody — a departed colleague — is left
// as literal "@{id}" text, which is exactly what the real renderer does and
// what bitbucket.ResolveMentions reads as "leave this one alone".
func renderHTML(raw string, accounts []bitbucket.Account) string {
	if raw == "" {
		return ""
	}

	// The escaping runs first: an account id is [A-Za-z0-9_|:-] and a brace is
	// not escaped either, so "@{id}" survives it byte for byte, while a "&" or
	// a "<" in the body is encoded before any span is spliced in.
	body := html.EscapeString(raw)
	for _, account := range accounts {
		if account.AccountID == "" {
			continue
		}
		body = strings.ReplaceAll(body, "@{"+account.AccountID+"}", mentionSpan(account))
	}
	return "<p>" + body + "</p>"
}

// mentionSpan is one rendered mention, in the shape Bitbucket writes it: a span
// keyed by data-atlassian-id whose text is "@" and the display name.
func mentionSpan(account bitbucket.Account) string {
	return `<span class="ap-mention" data-atlassian-id="` +
		html.EscapeString(account.AccountID) + `">@` +
		html.EscapeString(account.DisplayName) + `</span>`
}
