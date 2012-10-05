// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"flag"
	"github.com/stathat/stathatgo"
	"log"
)

type EZKey struct {
	Key string
}

func (e *EZKey) Count(name string, count int) {
	err := stathat.PostEZCount(name, e.Key, count)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
}

func (e *EZKey) Record(name string, value float64) {
	err := stathat.PostEZValue(name, e.Key, value)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
}

// Increment counter by 1.
func (e *EZKey) Inc(name string) {
	e.Count(name, 1)
}

// A Flag configured EZKey instance.
func EZKeyFlag(name string) *EZKey {
	e := &EZKey{}
	flag.StringVar(&e.Key, name+".key", "", name+" ezkey")
	return e
}
