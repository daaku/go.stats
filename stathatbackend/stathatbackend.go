// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
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
	Status   int    `json:"status"`
	Message  string `json:"msg"`
	Multiple int    `json:"multiple"`
}

type EZKey struct {
	Key                   string        // your StatHat EZ Key
	Debug                 bool          // enable logging of stat calls
	DialTimeout           time.Duration // timeout for net dial
	ResponseHeaderTimeout time.Duration // timeout for http read/write
	MaxIdleConns          int           // max idle http connections
	BatchTimeout          time.Duration // timeout for batching stats
	MaxBatchSize          int           // max items in a batch
	ChannelSize           int           // buffer size until we begin blocking
	stats                 chan interface{}
	closed                chan error
	client                *http.Client
}

func (e *EZKey) Count(name string, count int) {
	e.stats <- countStat{Name: name, Count: count}
}

func (e *EZKey) Record(name string, value float64) {
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
		case <-batchTimeout:
			go e.sendBatchLog(batch)
			batch = &apiRequest{EZKey: e.Key}
			batchTimeout = nil
		case stat, ok := <-e.stats:
			if e.Debug {
				if cs, ok := stat.(countStat); ok {
					log.Printf("stathatbackend: Count(%s, %d)", cs.Name, cs.Count)
				}
				if vs, ok := stat.(valueStat); ok {
					log.Printf("stathatbackend: Value(%s, %f)", vs.Name, vs.Value)
				}
			}
			if !ok {
				if e.Debug {
					log.Println("stathatbackend: process closed")
				}
				e.sendBatchLog(batch)
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
		}
	}
	close(e.closed)
}

func (e *EZKey) sendBatchLog(batch *apiRequest) {
	if err := e.sendBatch(batch); err != nil {
		log.Println(err)
	}
}

func (e *EZKey) sendBatch(batch *apiRequest) error {
	const url = "http://api.stathat.com/ez"
	if e.Debug {
		log.Printf("stathatbackend: sending batch with %d items", len(batch.Data))
	}
	j, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("stathatbackend: error json encoding request: %s", err)
	}
	if e.Debug {
		log.Printf("stathatbackend: request: %s", j)
	}
	resp, err := e.client.Post(url, "application/json", bytes.NewReader(j))
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
func (e *EZKey) Start() {
	e.stats = make(chan interface{}, e.ChannelSize)
	e.closed = make(chan error)
	e.client = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: func(network, addr string) (net.Conn, error) {
				return net.DialTimeout(network, addr, e.DialTimeout)
			},
			ResponseHeaderTimeout: e.ResponseHeaderTimeout,
			MaxIdleConnsPerHost:   e.MaxIdleConns,
		},
	}
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
		&e.DialTimeout,
		name+".http-dial-timeout",
		1*time.Second,
		name+" http dial timeout",
	)
	flag.DurationVar(
		&e.ResponseHeaderTimeout,
		name+".http-response-header-timeout",
		3*time.Second,
		name+" http response header timeout",
	)
	flag.IntVar(
		&e.MaxIdleConns,
		name+".max-idle-conns",
		10,
		name+" max idle connections to StatHat",
	)
	flag.DurationVar(
		&e.BatchTimeout,
		name+".batch-timeout",
		10*time.Second,
		name+" amount of time to aggregate a batch",
	)
	flag.IntVar(
		&e.MaxBatchSize,
		name+".max-batch-size",
		500,
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
