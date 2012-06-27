// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"github.com/stathat/stathatgo"
	"log"
)

type EZKey string

func (ezkey EZKey) Count(name string, count int) {
	err := stathat.PostEZCount(name, string(ezkey), count)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
}

func (ezkey EZKey) Record(name string, value float64) {
	err := stathat.PostEZValue(name, string(ezkey), value)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
}
