// Implements a stathat backend for go.stats.
package stathatbackend

import (
	"bytes"
	"encoding/json"
	"flag"
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
	DialTimeout           time.Duration // timeout for net dial (inc dns resolution)
	ResponseHeaderTimeout time.Duration // timeout for http read/write
	MaxIdleConns          int           // max idle http connections
	BatchTimeout          time.Duration // timeout for batching stats
	ChannelSize           int           // buffer size until we begin blocking
	stats                 chan interface{}
	closed                chan bool
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
	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered in EZKey.process: %v", r)
		}
	}()

	if e.Debug {
		log.Println("stathatbackend: started background process")
	}
	const url = "http://api.stathat.com/ez"
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: func(network, addr string) (net.Conn, error) {
				return net.DialTimeout(network, addr, e.DialTimeout)
			},
			ResponseHeaderTimeout: e.ResponseHeaderTimeout,
			MaxIdleConnsPerHost:   e.MaxIdleConns,
		},
	}

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
			if e.Debug {
				log.Printf("stathatbackend: request: %s", j)
			}
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
			err = json.NewDecoder(resp.Body).Decode(&apiResp)
			if err != nil {
				log.Printf("stathatbackend: error reading decoding response: %s", err)
			}
			if e.Debug {
				log.Printf("stathatbackend: api response: %+v", &apiResp)
			}
			if apiResp.Status != 200 {
				log.Printf("stathatbackend: api error: %+v", &apiResp)
			}
			resp.Body.Close()
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
				return
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
	e.stats = make(chan interface{}, e.ChannelSize)
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
		&e.ChannelSize,
		name+".channel-buffer-size",
		10000,
		name+" channel buffer size",
	)
	return e
}
