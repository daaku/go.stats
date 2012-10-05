// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"flag"
	"github.com/stathat/stathatgo"
	"log"
)

type EZKey struct {
	Key   string
	Debug bool
}

func (e *EZKey) Count(name string, count int) {
	err := stathat.PostEZCount(name, e.Key, count)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
	if e.Debug {
		log.Printf("stats.Count(%s, %d)", name, count)
	}
}

func (e *EZKey) Record(name string, value float64) {
	err := stathat.PostEZValue(name, e.Key, value)
	if err != nil {
		log.Printf("Failed to PostEZCount: %s", err)
	}
	if e.Debug {
		log.Printf("stats.Record(%s, %f)", name, value)
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
	flag.BoolVar(&e.Debug, name+".debug", false, name+" debug logging")
	return e
}
