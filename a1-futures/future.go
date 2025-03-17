// Package future provides a Future type that can be used to
// represent a value that will be available in the future.
package future

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// WeatherDataResult be used in GetWeatherData.
type WeatherDataResult struct {
	Value interface{}
	Err   error
}

type Future struct {
	result chan interface{}
}

func NewFuture() *Future {
	return &Future{
		result: make(chan interface{}, 1),
	}
}

// sets the future contents
func (f *Future) CompleteFuture(res interface{}) {
	f.result <- res
	f.CloseFuture()
}

// extacts and returns the contents of the future
// blocks until the contents are available
func (f *Future) GetResult() interface{} {
	return <-f.result
}

// closes the channel
func (f *Future) CloseFuture() {
	close(f.result)
}

// Wait waits for the first n futures to return or for the timeout to expire,
// whichever happens first.
func Wait(futures []*Future, n int, timeout time.Duration, postCompletionLogic func(interface{}) bool) []interface{} {
	var mu sync.Mutex

	doneN := make((chan bool))
	doneValues := make(chan interface{}, len(futures))
	doneCount := 0

	for i := 0; i < len(futures); i++ {
		go func(i int) {
			val := futures[i].GetResult()
			if postCompletionLogic == nil || postCompletionLogic(val) {
				doneValues <- val
				mu.Lock()
				doneCount++
				if doneCount >= n {
					doneN <- true
				}
				mu.Unlock()
			}

		}(i)
	}

	chanToSlice := func(ch chan interface{}, n int) []interface{} {
		s := make([]interface{}, 0)
		min := func(a, b int) int {
			if a < b {
				return a
			}
			return b
		}
		l := min(n, len(ch))
		for i := 0; i < l; i++ {
			s = append(s, <-ch)
		}
		return s
	}

	select {
	case <-doneN:
		return chanToSlice(doneValues, n)
	case <-time.After(timeout):
		return chanToSlice(doneValues, n)
	}

}

// User Defined Function Logic

// GetWeatherData implementation which immediately returns a Future.
func GetWeatherData(baseURL string, id int) *Future {
	f := NewFuture()
	go func() {
		var temp float64
		var err error

		resp, err := http.Get(fmt.Sprintf("%s/weather?id=%d", baseURL, id))

		if err == nil {
			var body []byte
			defer resp.Body.Close()
			body, err = io.ReadAll(resp.Body)
			if err == nil {
				err = json.Unmarshal(body, &temp)
			}
		}

		f.CompleteFuture(WeatherDataResult{Value: temp, Err: err})
	}()
	return f
}

// heatWaveWarning is the PostCompletionLogic function for the received weatherData.
// Should be used to filter out all temperatures > 35 degrees Celsius.
func heatWaveWarning(res interface{}) bool {
	// TODO: Your code here
	weatherData := res.(WeatherDataResult)
	if weatherData.Err != nil {
		return false
	}

	temp := weatherData.Value.(float64)
	return temp > 35
}
