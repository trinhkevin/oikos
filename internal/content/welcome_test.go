package content

import (
	"strings"
	"testing"
)

func TestWelcomeLoaderRendersMarkdown(t *testing.T) {
	loader := NewWelcomeLoader(NewCache())
	html, err := loader.Load("testdata/welcome/hello.md")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(html, "<p>") {
		t.Errorf("html = %q, want a rendered paragraph", html)
	}
	if !strings.Contains(html, "welcome") {
		t.Errorf("html = %q, want it to contain fixture text", html)
	}
}
