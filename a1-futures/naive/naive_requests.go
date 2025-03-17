package main

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// GetWeatherData simulates slow GetWeatherData request that takes some
// time before it returns a value. This is blocking.
func GetWeatherData(straggler bool, unreliable bool) (int, bool) {

	// Simulate an asynchronous operation.
	time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)

	if straggler {
		// Sleeps for longer, but will eventually complete.
		time.Sleep(time.Duration(100+rand.Intn(500)) * time.Millisecond)
	}

	if unreliable {
		// The future will never complete.
		// We must close the channel to return a <nil>,
		// else the future will never return anything.
		return -1, true
	}

	// Get random number
	randomNumber := rand.Intn(10000)
	return randomNumber, false
}

// Once a majority (nPeers/2 + 1) GetWeatherData calls return a value
// we can stop waiting for the remaining go routines.
// If 250 ms have elapsed, we stop waiting for the go routines regardless
// of how many have returned a value.
func main() {
	var mu sync.Mutex
	fmt.Printf("Naive Requests Demo\n")

	done := make(chan bool)
	nPeers := 10
	requiredTrueCount := (nPeers / 2) + 1
	trueCount := 0
	timeout := 200 * time.Millisecond

	// Get value of SlowFunction from all peers in a concurrent manner.
	for i := 0; i < nPeers; i++ {
		go func() {
			_, err := GetWeatherData(false, false)
			if !err {
				mu.Lock()
				trueCount++
				if trueCount >= requiredTrueCount {
					done <- true
				}
				mu.Unlock()
			}
		}()
	}

	// Stop once (nPeers/2 + 1) responses have been received or the function times out.
	select {
	case <-done:
		fmt.Println("Done")
	case <-time.After(timeout):
		fmt.Println("Timing out", timeout)
	}

	fmt.Printf("Total true values received: %v\n", trueCount)
}
