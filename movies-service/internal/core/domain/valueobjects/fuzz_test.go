package valueobjects_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	errD "movies-service/internal/core/domain/err-d"
	"movies-service/internal/core/domain/valueobjects"
)

// FuzzNewMovieTitle checks the invariant of NewMovieTitle for any input:
// it accepts a title iff its trimmed form is non-empty and <= 300 bytes,
// and never panics.
func FuzzNewMovieTitle(f *testing.F) {
	for _, seed := range []string{"Inception", "", "   ", strings.Repeat("a", 300), strings.Repeat("a", 301), "\t\n", "Amélie 🎬"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, title string) {
		got, err := valueobjects.NewMovieTitle(title)

		trimmed := strings.TrimSpace(title)
		wantValid := trimmed != "" && len(trimmed) <= 300

		if wantValid {
			if err != nil {
				t.Fatalf("NewMovieTitle(%q) rejeitou título válido: %v", title, err)
			}
			if got.Title != title {
				t.Fatalf("NewMovieTitle(%q) alterou o título para %q", title, got.Title)
			}
			return
		}
		if err != errD.ErrTitleNotValid {
			t.Fatalf("NewMovieTitle(%q) err = %v, esperado ErrTitleNotValid", title, err)
		}
	})
}

// FuzzNewMovieYear checks that NewMovieYear accepts exactly the strings
// strconv.Atoi parses into [1888, ano atual + 2].
func FuzzNewMovieYear(f *testing.F) {
	for _, seed := range []string{"2010", "1888", "1887", "0", "-1", "abc", "", " 2010", "+2010", "99999999999999999999"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, year string) {
		_, err := valueobjects.NewMovieYear(year)

		n, convErr := strconv.Atoi(year)
		wantValid := convErr == nil && n >= 1888 && n <= time.Now().Year()+2

		if wantValid && err != nil {
			t.Fatalf("NewMovieYear(%q) rejeitou ano válido: %v", year, err)
		}
		if !wantValid && err != errD.ErrYearNotValid {
			t.Fatalf("NewMovieYear(%q) err = %v, esperado ErrYearNotValid", year, err)
		}
	})
}

func FuzzNewMovieID(f *testing.F) {
	for _, seed := range []int32{1, 0, -1, 2147483647, -2147483648} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, id int32) {
		got, err := valueobjects.NewMovieID(id)
		if id > 0 {
			if err != nil || got.ID() != id {
				t.Fatalf("NewMovieID(%d) = %v, %v", id, got, err)
			}
			return
		}
		if err != errD.ErrIDNotValid {
			t.Fatalf("NewMovieID(%d) err = %v, esperado ErrIDNotValid", id, err)
		}
	})
}

func BenchmarkNewMovieTitle(b *testing.B) {
	for b.Loop() {
		_, _ = valueobjects.NewMovieTitle("The Lord of the Rings: The Fellowship of the Ring")
	}
}

func BenchmarkNewMovieYear(b *testing.B) {
	for b.Loop() {
		_, _ = valueobjects.NewMovieYear("2010")
	}
}

func BenchmarkNewMovieID(b *testing.B) {
	for b.Loop() {
		_, _ = valueobjects.NewMovieID(68152)
	}
}
