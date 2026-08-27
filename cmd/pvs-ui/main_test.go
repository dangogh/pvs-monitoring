package main

import (
	"strings"
	"testing"

	iofs "io/fs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The markers are exact literals. If index.html's title or heading is reworded,
// substitution silently stops happening — this catches that at build time.
func TestMarkersPresentInEmbeddedPage(t *testing.T) {
	staticFS, err := iofs.Sub(staticFiles, "static")
	require.NoError(t, err)
	page, err := iofs.ReadFile(staticFS, "index.html")
	require.NoError(t, err)

	assert.Contains(t, string(page), titleMarker, "title marker must match index.html")
	assert.Contains(t, string(page), headingMarker, "heading marker must match index.html")
}

func TestWithHostname(t *testing.T) {
	staticFS, err := iofs.Sub(staticFiles, "static")
	require.NoError(t, err)
	page, err := iofs.ReadFile(staticFS, "index.html")
	require.NoError(t, err)

	t.Run("substitutes title and heading", func(t *testing.T) {
		got := string(withHostname(page, "helios"))
		assert.Contains(t, got, "<title>Solar Monitor: helios</title>")
		assert.Contains(t, got, "<h1>☀ Solar Monitor: helios</h1>")
		assert.NotContains(t, got, titleMarker)
		assert.NotContains(t, got, headingMarker)
	})

	t.Run("empty hostname leaves the page untouched", func(t *testing.T) {
		got := withHostname(page, "")
		assert.Equal(t, string(page), string(got))
	})

	t.Run("escapes HTML metacharacters in the hostname", func(t *testing.T) {
		got := string(withHostname(page, `a<script>b`))
		assert.NotContains(t, got, "<title>Solar Monitor: a<script>")
		assert.Contains(t, got, "a&lt;script&gt;b")
	})

	t.Run("substitutes each marker only once", func(t *testing.T) {
		got := string(withHostname(page, "helios"))
		assert.Equal(t, 1, strings.Count(got, "<title>Solar Monitor: helios</title>"))
		assert.Equal(t, 1, strings.Count(got, "<h1>☀ Solar Monitor: helios</h1>"))
	})
}

func TestShortHostname(t *testing.T) {
	// Can't control the real hostname, but it must never contain a dot or be a
	// bare domain fragment.
	got := shortHostname()
	assert.NotContains(t, got, ".", "domain suffix should be trimmed")
}
