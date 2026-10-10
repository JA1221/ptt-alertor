// Package redistest runs package tests against an in-memory redis
package redistest

import (
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

var server *miniredis.Miniredis

// Run starts miniredis, points redis connections to it and runs tests,
// use it in TestMain: os.Exit(redistest.Run(m))
func Run(m *testing.M) int {
	s, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	defer s.Close()
	server = s
	os.Setenv("REDIS_ENDPOINT", s.Host())
	os.Setenv("REDIS_PORT", s.Port())
	return m.Run()
}

// Flush clears all data
func Flush() {
	server.FlushAll()
}
