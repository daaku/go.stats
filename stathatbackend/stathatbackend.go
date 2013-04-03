// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/mreiferson/go-httpclient"
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
	Data  []interface{} `json:"data"`
}

type apiResponse struct {
	Status  int    `json:"status"`
	Message string `json:"msg"`
}

type EZKey struct {
	Key              string        // your StatHat EZ Key
	Debug            bool          // enable logging of stat calls
	ConnectTimeout   time.Duration // timeout for http connect
	ReadWriteTimeout time.Duration // timeout for http read/write
	MaxConnections   int           // max simultaneous http connections
	BatchTimeout     time.Duration // timeout for batching stats
	BufferSize       int           // buffer size until we begin blocking
	stats            chan interface{}
	closed           chan bool
}

func (e *EZKey) Count(name string, count int) {
	e.stats <- countStat{Name: name, Count: count}
	if e.Debug {
		log.Printf("stats.Count(%s, %d)", name, count)
	}
}

func (e *EZKey) Record(name string, value float64) {
	e.stats <- valueStat{Name: name, Value: value}
	if e.Debug {
		log.Printf("stats.Record(%s, %f)", name, value)
	}
}

// Increment counter by 1.
func (e *EZKey) Inc(name string) {
	e.Count(name, 1)
}

// Actually send the stats to stathat.
func (e *EZKey) process() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered in EZKey.process: %v", r)
		}
	}()

	if e.Debug {
		log.Println("stathatbackend: started background process")
	}
	const url = "http://api.stathat.com/ez"
	client := httpclient.New()
	client.ConnectTimeout = e.ConnectTimeout
	client.ReadWriteTimeout = e.ReadWriteTimeout
	client.MaxConnsPerHost = e.MaxConnections
	client.Verbose = e.Debug

	var batchTimeout <-chan time.Time
	batch := &apiRequest{EZKey: e.Key}
	apiResp := apiResponse{}
	for {
		select {
		case <-batchTimeout:
			if e.Debug {
				log.Printf("stathatbackend: sending batch with %d items", len(batch.Data))
			}
			j, err := json.Marshal(batch)
			if err != nil {
				log.Printf("stathatbackend: error json encoding request: %s", err)
				continue
			}
			log.Printf("json: %s", j)
			batch.Data = nil
			batchTimeout = nil
			req, err := http.NewRequest("POST", url, bytes.NewReader(j))
			if err != nil {
				log.Printf("stathatbackend: error creating request: %s", err)
				continue
			}
			req.Header.Add("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err != nil {
				log.Printf("stathatbackend: error performing request: %s", err)
				continue
			}
			defer resp.Body.Close()
			defer client.FinishRequest(req)
			if resp.StatusCode != 200 {
				log.Printf("stathatbackend: non 200 response: %d", resp.StatusCode)
				continue
			}
			err = json.NewDecoder(resp.Body).Decode(&apiResp)
			if err != nil {
				log.Printf("stathatbackend: error reading decoding response: %s", err)
				continue
			}
			if apiResp.Status != 200 {
				log.Printf("stathatbackend: api error: %v", apiResp)
				continue
			}
		case stat, ok := <-e.stats:
			if !ok {
				if e.Debug {
					log.Println("stathatbackend: process closed")
				}
				return
			}
			if e.Debug {
				log.Println("stathatbackend: got stat")
			}
			batch.Data = append(batch.Data, stat)
			if batchTimeout == nil {
				batchTimeout = time.After(e.BatchTimeout)
			}
		}
	}
	close(e.closed)
}

// Start the background goroutine for handling the actual HTTP requests.
func (e *EZKey) Start() {
	e.stats = make(chan interface{}, e.BufferSize)
	e.closed = make(chan bool)
	go e.process()
}

// Close the background goroutine.
func (e *EZKey) Close() error {
	close(e.stats)
	<-e.closed
	return nil
}

// A Flag configured EZKey instance.
func EZKeyFlag(name string) *EZKey {
	e := &EZKey{}
	flag.StringVar(&e.Key, name+".key", "", name+" ezkey")
	flag.BoolVar(&e.Debug, name+".debug", false, name+" debug logging")
	flag.DurationVar(
		&e.ConnectTimeout,
		name+".http-connect-timeout",
		1*time.Second,
		name+" http connect timeout",
	)
	flag.DurationVar(
		&e.ReadWriteTimeout,
		name+".http-read-write-timeout",
		10*time.Second,
		name+" http read/write timeout",
	)
	flag.DurationVar(
		&e.BatchTimeout,
		name+".batch-timeout",
		10*time.Second,
		name+" batch timeout",
	)
	flag.IntVar(
		&e.BufferSize,
		name+".buffer-size",
		10000,
		name+" buffer size",
	)
	return e
}
