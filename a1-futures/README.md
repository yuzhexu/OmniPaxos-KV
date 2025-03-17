# Assignment 1: Futures in Go

**Due: Fri, Sept 20, 11:59 PM**

Please make sure to regularly commit and push your work to gitlab. In case of any submission related issues, your last commit before the deadline will be treated as your submission.

## Futures
In concurrent programming, a `Future` is an object that acts as a placeholder for a result that is initially unknown but will be computed asynchronously. In this library, we provide a `Future` type and function headers for a `Wait` function. The `Wait` function returns a slice containing the values for the Futures passed into it, once the Futures are `Completed`.

We will describe how to implement this in more detail later in this document.

## Motivation
You might be wondering why we need to implement such a library, when we can achieve the same in a naive fashion. To demonstrate this, we provide you with a file [naive/naive_requests.go](naive/naive_requests.go). This implements a slow function, called `GetWeatherData` and its driver code to collect temperature readings in a "non-Future" manner.

You can run this using the following command:
```shell
go run naive/naive_requests.go
```

`GetWeatherData` is non-deterministic, since it returns a random output, and it's internal delays are randomized, you might see a different output on every run. 

Some example outputs are:
```shell
Naive Requests Demo
Timing out 250ms
Total true values received: 4
```
```shell
Naive Requests Demo
Done
Total true values received: 6
```

You should carefully read the code and the corresponding comments explaining the logic.

## Your Task

### Task 1
Your first task is to implement the `Wait` function.

```go 
func Wait(futures []*Future, n int, timeout time.Duration, postCompletionLogic func(interface{}) bool) []interface{}
```

#### Parameters
`futures []*Future`: A slice of `Future` objects to wait on.

`n int`: The number of futures to wait for.

`timeout time.Duration`: The maximum time to wait before timing out.

`postCompletionLogic func(interface{}) bool`: A function to check each future’s result. Returns true if the result meets the required condition.

#### Returns
`[]interface{}`: A slice of results from the first n futures that completed and met the condition specified by postCompletionLogic.

#### Description
This function will wait for the first `n` futures to return or until a specified `timeout` occurs. You will also implement logic to check each future’s result using a provided function, `postCompletionLogic`.

---
### Task 2
Once you have a basic `Wait` function in place, you need to implement a user defined function that returns a Future. Specifically, you would be implementing the `GetWeatherData` function.
```go
  func GetWeatherData(baseURL string, id int) *Future
```

#### Parameters
`baseURL string`: The URL of the server where you will make the `GET` request.

`id`: The id of the weather station you'll be getting the data for.

#### Returns
`*Future`: A future that can be used to access the results of the API request.

#### Description

`GetWeatherData` is responsible for making a request to an HTTP server. The URL for the server is received as the function parameter `baseURL`. You will be making a `GET` request to `baseURL/weather?id=<id>`. 

For example, if the `baseURL` is `localhost:3000` and the weather station `id` is `3`, you need to make a request to `localhost:3000/weather?id=3`. 

This API request will return a Value of type `float64`. For example `75.6`.

The result of the API request should be stored in a struct of type `WeatherDataResult`, which you have already been provided with. The function should return a `Future` immediately, and then, in the background, work on actually getting the data. 

`GetWeatherData` will call the `CompleteFuture()` function once it's finished processing. The result of the `GetWeatherData` can then be accessed via calling `GetResult()` on the returned `Future` object.

The tester expects a `Future` object as the return value of `GetWeatherData` and will fail if you don't do so.

Note that you must ensure that you return a slice of all the results from the completed futures. 

Make sure that you start early so that you have enough time to weed out all possible bugs.



## Observations
Take a look at the code required to get the values from `GetWeatherData` in [naive_requests.go](naive/naive_requests.go).

You will notice that with the library you just implemented, the about 20 lines of code in the naive approach are reduced to the following in [future_test.go](future_test.go)!

```go
results := make([]*Future, 0)
nPeers := 10
for i := 0; i < nPeers; i++ {
  results = append(results, slowFunction(false, false))
}

returned_results := Wait(results, (nPeers/2)+1, 300*time.Millisecond, nil)
```

Pretty cool, right?

## Testing your code
You can test your code by running the following command:
```
go test -v -race
```

You can also run individual test cases by running:
```
go test -v -race -run TestFutureBasic
```
Replace "TestFutureBasic" with the name of the test you're trying to run.

On successful completion, you should see something similar to this output
```
=== RUN   TestFutureBasic
--- PASS: TestFutureBasic (0.22s)
=== RUN   TestFutureTimeout
--- PASS: TestFutureTimeout (0.30s)
=== RUN   TestFuturePostCompletionLogic
--- PASS: TestFuturePostCompletionLogic (0.30s)
=== RUN   TestFutureUnreliable
--- PASS: TestFutureUnreliable (0.24s)
=== RUN   TestGetWeatherDataBasic
--- PASS: TestGetWeatherDataBasic (0.21s)
=== RUN   TestGetWeatherDataDelayedResponses
--- PASS: TestGetWeatherDataDelayedResponses (0.21s)
=== RUN   TestGetWeatherDataOneFail
--- PASS: TestGetWeatherDataOneFail (0.20s)
PASS
ok  	cs651/a1-futures	2.856s
```

## Submission

Upload the `future.go` file to Gradescope. Please do not change any of the function signatures, or the autograder may fail to run.

All the best!