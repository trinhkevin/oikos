package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWelcomePageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Brivin Household") {
		t.Error("expected page to contain site title")
	}
	if !strings.Contains(rec.Body.String(), "Wi-Fi") {
		t.Error("expected welcome hub to link to Wi-Fi")
	}
}

func TestCoffeeMenuPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/coffee", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Espresso") {
		t.Error("expected coffee menu content in response")
	}
}

func TestCocktailsMenuPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cocktails", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Negroni") {
		t.Error("expected cocktail menu content in response")
	}
}

func TestRefreshmentsPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/refreshments", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Mixed Nuts") {
		t.Error("expected refreshments content in response")
	}
}

// Note: the plan's brief for this test used the placeholder cat "Mochi",
// but Task 4 substituted real seed cats (Farah and Zeke) for the brief's
// placeholder content (Mochi/Biscuit) per controller instruction — see
// progress.md and task-4-report.md. These assertions target the real seed
// content actually present under content/cats/.
func TestCatsIndexPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Farah") {
		t.Error("expected cats index to list Farah")
	}
}

func TestCatDetailPageRenders(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats/farah", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "churu") {
		t.Error("expected cat detail page to render Farah's likes")
	}
}

func TestCatDetailPageUnknownSlugReturns404(t *testing.T) {
	s := New(testConfig())
	req := httptest.NewRequest(http.MethodGet, "/cats/no-such-cat", nil)
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
