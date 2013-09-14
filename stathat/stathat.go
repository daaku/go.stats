// Package stathat implements a stathat backend for go.stats.
package stathat

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/daaku/go.jsonpipe"
)

type countStat struct {
	Name  string `json:"stat"`
	Count int    `json:"count"`
}

type valueStat struct {
	Name  string  `json:"stat"`
	Value float64 `json:"value"`
}

type apiRequest struct {
	EZKey string        `json:"ezkey"`
	Data  []interface{} `json:"data"` // countStat or valueStat
}

type apiResponse struct {
	Status   int    `json:"status"`
	Message  string `json:"msg"`
	Multiple int    `json:"multiple"`
}

type EZKey struct {
	Key          string        // your StatHat EZ Key
	Debug        bool          // enable logging of stat calls
	BatchTimeout time.Duration // timeout for batching stats
	MaxBatchSize int           // max items in a batch
	ChannelSize  int           // buffer size until we begin blocking
	Transport    http.RoundTripper
	stats        chan interface{} // countStat or valueStat
	closed       chan error
	startOnce    sync.Once
}

func (e *EZKey) Count(name string, count int) {
	e.startOnce.Do(e.start)
	e.stats <- countStat{Name: name, Count: count}
}

func (e *EZKey) Record(name string, value float64) {
	e.startOnce.Do(e.start)
	e.stats <- valueStat{Name: name, Value: value}
}

func (e *EZKey) Inc(name string) {
	e.Count(name, 1)
}

// Actually send the stats to stathat.
func (e *EZKey) process() {
	if e.Debug {
		log.Println("stathatbackend: started background process")
	}

	var batchTimeout <-chan time.Time
	batch := &apiRequest{EZKey: e.Key}
	for {
		select {
		case stat, open := <-e.stats:
			if e.Debug {
				if cs, ok := stat.(countStat); ok {
					log.Printf("stathatbackend: Count(%s, %d)", cs.Name, cs.Count)
				}
				if vs, ok := stat.(valueStat); ok {
					log.Printf("stathatbackend: Value(%s, %f)", vs.Name, vs.Value)
				}
			}
			if !open {
				if e.Debug {
					log.Println("stathatbackend: process closed")
				}
				e.sendBatchLog(batch)
				close(e.closed)
				return
			}
			batch.Data = append(batch.Data, stat)
			if batchTimeout == nil {
				batchTimeout = time.After(e.BatchTimeout)
			}
			if len(batch.Data) >= e.MaxBatchSize {
				go e.sendBatchLog(batch)
				batch = &apiRequest{EZKey: e.Key}
				batchTimeout = nil
			}
		case <-batchTimeout:
			go e.sendBatchLog(batch)
			batch = &apiRequest{EZKey: e.Key}
			batchTimeout = nil
		}
	}
}

func (e *EZKey) sendBatchLog(batch *apiRequest) {
	if err := e.sendBatch(batch); err != nil {
		log.Println(err)
	}
}

func (e *EZKey) sendBatch(batch *apiRequest) error {
	if len(batch.Data) == 0 {
		return nil
	}
	if e.Debug {
		log.Printf("stathatbackend: sending batch with %d items", len(batch.Data))
	}

	const url = "http://api.stathat.com/ez"
	req, err := http.NewRequest("POST", url, jsonpipe.Encode(batch))
	if err != nil {
		return fmt.Errorf("stathatbackend: error creating http request: %s", err)
	}
	req.Header.Add("Content-Type", "application/json")

	resp, err := e.Transport.RoundTrip(req)
	if err != nil {
		return fmt.Errorf("stathatbackend: %s", err)
	}
	defer resp.Body.Close()
	var apiResp apiResponse
	err = json.NewDecoder(resp.Body).Decode(&apiResp)
	if err != nil {
		return fmt.Errorf("stathatbackend: error decoding response: %s", err)
	}
	if apiResp.Status != 200 {
		return fmt.Errorf("stathatbackend: api error: %+v", &apiResp)
	} else if e.Debug {
		log.Printf("stathatbackend: api response: %+v", &apiResp)
	}
	return nil
}

// Start the background goroutine for handling the actual HTTP requests.
func (e *EZKey) start() {
	e.stats = make(chan interface{}, e.ChannelSize)
	e.closed = make(chan error)
	go e.process()
}

// Close the background goroutine.
func (e *EZKey) Close() error {
	close(e.stats)
	return <-e.closed
}

// A Flag configured EZKey instance.
func EZKeyFlag(name string) *EZKey {
	e := &EZKey{}
	flag.StringVar(&e.Key, name+".key", "", name+" ezkey")
	flag.BoolVar(&e.Debug, name+".debug", false, name+" debug logging")
	flag.DurationVar(
		&e.BatchTimeout,
		name+".batch-timeout",
		10*time.Second,
		name+" amount of time to aggregate a batch",
	)
	flag.IntVar(
		&e.MaxBatchSize,
		name+".max-batch-size",
		1000,
		name+" maximum number of items in a batch",
	)
	flag.IntVar(
		&e.ChannelSize,
		name+".channel-buffer-size",
		10000,
		name+" channel buffer size",
	)
	return e
}
