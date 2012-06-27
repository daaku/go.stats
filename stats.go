// Package stats provides a simple indirection layer to make it easy
// to add stats without locking into a specific backend.
package stats

import (
	"flag"
	"log"
)

// Defines the backend implementation.
type Backend interface {
	Count(name string, count int)
	Record(name string, value float64)
}

var (
	verbose = flag.Bool("stats.verbose", false, "Enable verbose logging.")
	backend Backend
)

// Set the stats backend.
func SetBackend(b Backend) {
	backend = b
}

// Record a value.
func Record(name string, value float64) {
	if *verbose {
		log.Printf("stats.Value(%s, %f)", name, value)
	}
	backend.Record(name, value)
}

// Increment counter by given value.
func Count(name string, count int) {
	if *verbose {
		log.Printf("stats.Count(%s, %d)", name, count)
	}
	backend.Count(name, count)
}

// Increment counter by 1.
func Inc(name string) {
	Count(name, 1)
}
