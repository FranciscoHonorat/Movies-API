package entity_test

import (
	"testing"

	"movies-service/internal/core/domain/entity"
)

func BenchmarkNewMovieEntity(b *testing.B) {
	for b.Loop() {
		_, _ = entity.NewMovieEntity(68152, "Inception", "2010")
	}
}

func BenchmarkNewMovieEntityForCreate(b *testing.B) {
	for b.Loop() {
		_, _ = entity.NewMovieEntityForCreate("Inception", "2010")
	}
}
