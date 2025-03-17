# Lab 3: Fault-tolerant Key-Value Store


## Introduction

In this lab you will build a fault-tolerant key-value store using your OmniPaxos library from Lab2. Your key-value store will consist of several key-value servers that use OmniPaxos for replication and fault-tolerance. Your key-value store should continue to process client requests as long as a majority of servers are alive and can communicate, even in the case of failures or network partitions. After Lab 3, you will have implemented all parts (Clerk, Service, and OmniPaxos) shown in the [diagram of OmniPaxos interactions](./KVPaxos.png).

Clients can send three different RPCs to the key-value store: `Put(key, value)`, `Append(key, arg)`, and `Get(key)`. The store maintains a simple database of key-value pairs. Keys and values are strings. `Put(key, value)` replaces the value for a particular key in the database, `Append(key, arg)` appends arg to key's value, and `Get(key)` fetches the current value for the key. A Get for a non-existent key should return an empty string. An Append to a non-existent key should act like Put. Each client talks to the service through a Clerk with `Put/Append/Get` methods. A Clerk manages RPC interactions with the servers.

Your store must guarantee that client requests (`Get/Put/Append`) are linearizable. If called one at a time, the Get/Put/Append methods should act as if the system had only one copy of its state, and each call should observe the modifications to the state implied by the preceding sequence of calls. For concurrent calls, the return values and final state must be the same as if the operations had executed one at a time in some order. Calls are concurrent if they overlap in time: for example, if client X calls `Clerk.Put()` and client Y call `Clerk.Append()` before X's call returns, the two Put and Append requests are concurrent. A call must observe the effects of all calls that have completed before the call starts (or the effect of a concurrent call).

Start early.

## Getting Started

We supply you with skeleton code and tests in `src/kvpaxos`. You will need to modify `kvpaxos/client.go, kvpaxos/server.go`, and perhaps `kvpaxos/common.go`.

To get up and running, execute the following commands. **Don't forget the git pull to get the latest software.**

```
$ cd ~/cs651
$ git pull
...
$ cd a3-kvpaxos
$ go test
...
$
```

## Key-value store

Each of your key-value servers ("kvservers") will have an associated OmniPaxos peer. Clerks send `Put(), Append(), and Get()` RPCs to the kvserver whose associated OmniPaxos is the leader. The kvserver code submits the `Put/Append/Get` operation to OmniPaxos, so that the OmniPaxos log holds a sequence of Put/Append/Get operations. All of the kvservers execute operations from the OmniPaxos log in order, applying the operations to their key-value databases; the goal is to maintain identical replicas of the key-value database.

A Clerk sometimes doesn't know which kvserver is the OmniPaxos leader. If the Clerk sends an RPC to the wrong kvserver, or if it cannot reach the kvserver, the Clerk should re-try by sending to a different kvserver (e.g., in a round-robin fashion). If the service "decides" (i.e., commits) the operation in the OmniPaxos log and applies it to the database, the leader reports the result to the Clerk by responding to its RPC. If the operation failed to commit (for example, if the leader was replaced), the server reports an error, and the Clerk retries with a different server.

**Your kvservers should not directly communicate; they should only interact with each other through OmniPaxos.**

Your first task is to implement a solution that works when there are no dropped messages and no failed servers.

You'll need to add RPC-sending code to the Clerk Put/Append/Get methods in client.go, and implement `PutAppend()` and `Get()` RPC handlers in server.go. These handlers should enter an Op in the OmniPaxos log using `Proposal()`; you should fill in the Op struct definition in `server.go` so that it describes a `Put/Append/Get` operation. Each server should execute Op commands as OmniPaxos commits them, i.e. as they appear on the applyCh. An RPC handler should notice when OmniPaxos commits its Op, and then reply to the RPC.

You have completed this task when you **reliably** pass the first test in the test suite: "One client".

*   After calling `Proposal()`, your kvservers will need to wait for OmniPaxos to complete agreement. Commands that have been agreed upon arrive on the applyCh. Your code will need to keep reading applyCh while `PutAppend()` and `Get()` handlers submit commands to the OmniPaxos log using `Proposal()`. Beware of deadlock between the kvserver and its OmniPaxos library.
*   You are allowed to add fields to the OmniPaxos ApplyMsg, and to add fields to OmniPaxos RPCs such as Accept, however this should not be necessary for most implementations.
*   A kvserver should not complete a `Get()` RPC if it is not part of a majority (so that it does not serve stale data). A simple solution is to enter every `Get()` (as well as each `Put()` and `Append()`) in the OmniPaxos log. 
*   It's best to add locking from the start because the need to avoid deadlocks sometimes affects overall code design. Check that your code is race-free using go test -race.

At this point, you should modify your solution to continue in the face of network and server failures. One problem you'll face is that a Clerk may have to send an RPC multiple times until it finds a kvserver that replies positively. If a leader fails just after committing an entry to the OmniPaxos log, the Clerk may not receive a reply, and thus may re-send the request to another leader. Each call to `Clerk.Put()` or `Clerk.Append()` should result in just a single execution, so you will have to ensure that the re-send doesn't result in the servers executing the request twice.

Add code to handle failures, and to cope with duplicate Clerk requests, including situations where the Clerk sends a request to a kvserver leader in one term, times out waiting for a reply, and re-sends the request to a new leader in another term. The request should execute just once. These notes include guidance on [duplicate detection](./Notes.md). Your code should pass the `go test -run 3A` tests.

*   Your solution needs to handle a leader that has called `Proposal()` for a Clerk's RPC, but loses its leadership before the request is committed to the log. In this case you should arrange for the Clerk to re-send the request to other servers until it finds the new leader. One way to do this is for the server to detect that it has lost leadership, by noticing that a different request has appeared at the index returned by `Proposal()`, or that OmniPaxos's term has changed. If the ex-leader is disconnected, it won't know about new leaders; but any client in the same partition won't be able to talk to a new leader either, so it's OK in this case for the server and client to wait indefinitely until the network partition is resolved.
*   You will probably have to modify your Clerk to remember which server turned out to be the leader for the last RPC, and send the next RPC to that server first. This will avoid wasting time searching for the leader on every RPC, which may help you pass some of the tests quickly enough.
*   You will need to uniquely identify client operations to ensure that the key-value service executes each one just once.
*   Your scheme for duplicate detection should free server memory quickly, for example by having each RPC imply that the client has seen the reply for its previous RPC. It's OK to assume that a client will make only one call into a Clerk at a time.

Your code should now pass the Lab 3A tests, like this:
```
$ go test -race -loglevel 6
Test:  Basic Test One Client ...
  ... Passed --   1.8  3  2826  306
Test:  Basic Test Many Client ...
  ... Passed --  13.4  3  8767  918
Test:  Unreliable Test Network ...
  ... Passed --   1.8  3   215   15
Test:  Basic Fig 5A One Client ...
  ... Passed --  35.3  5 27362  918
Test:  Basic Fig 5A Many Client ...
  ... Passed --   6.7  5  6330  216
Test:  Basic Fig 5B One Client ...
  ... Passed --  20.2  5 16820  678
Test:  Basic Fig 5B Many Client ...
  ... Passed --   6.8  5  5386  216
Test:  Basic Fig 5C One Client ...
  ... Passed --  11.9  3  6570  678
Test:  Basic Fig 5C Many Client ...
  ... Passed --   5.0  3  2164  216
Test:  Basic Persist One Client ...
  ... Passed --   3.8  5  5450  192
Test:  Basic Persist Many Client ...
  ... Passed --  15.5  5 16924  576
PASS
ok  	cs651/a3-kvpaxos	128.876s
```
The numbers after each Passed are real time in seconds, number of peers, number of RPCs sent (including client RPCs), and number of key-value operations executed (Clerk Get/Put/Append calls).
