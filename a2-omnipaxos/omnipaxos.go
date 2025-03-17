package omnipaxos

//
// This is an outline of the API that OmniPaxos must expose to
// the service (or tester). See comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   Create a new OmniPaxos server.
// op.Start(command interface{}) (index, ballot, isleader)
//   Start agreement on a new log entry
// op.GetState() (ballot, isLeader)
//   ask a OmniPaxos for its current ballot, and whether it thinks it is leader
// ApplyMsg
//   Each time a new entry is committed to the log, each OmniPaxos peer
//   should send an ApplyMsg to the service (or tester) in the same server.
//

import (
	"bytes"
	"encoding/gob"
	"sync"
	"sync/atomic"
	"time"

	"cs651/labrpc"
)

// A Go object implementing a single OmniPaxos peer.
type OmniPaxos struct {
	mu            sync.Mutex          // Lock to protect shared access to this peer's state
	peers         []*labrpc.ClientEnd // RPC end points of all peers
	persister     *Persister          // Object to hold this peer's persisted state
	me            int                 // This peer's index into peers[]
	dead          int32               // Set by Kill()
	enableLogging int32
	// Your code here (2A, 2B).
	currentRnd Ballot
	promises   map[int]*Promise
	maxProm    *Promise
	accepted   []int
	buffer     []interface{}

	os OmniPaxosState

	r       int
	b       Ballot
	qc      bool
	delay   time.Duration
	ballots map[Ballot]bool

	state State

	applyCh chan ApplyMsg

	linkDrop       bool
	missedHbCounts map[int]int

	restart Restart
}

type Restart struct {
	loop int
}

type Promise struct {
	f      int
	accRnd Ballot
	logIdx int
	decIdx int
	log    []interface{}
}

// As each OmniPaxos peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). Set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

// Ensure that all your fields for a RCP start with an Upper Case letter
type HBRequest struct {
	// Your code here (2A).
	Rnd int
}

type HBReply struct {
	// Your code here (2A).
	Rnd    int
	Ballot int
	Q      bool
}

type PrepareRequest struct {
	L      int
	N      Ballot
	AccRnd Ballot
	LogIdx int
	DecIdx int
}

type DummyReply struct{}

type PromiseRequest struct {
	F      int
	N      Ballot
	AccRnd Ballot
	LogIdx int
	DecIdx int
	Sfx    []interface{}
}

type AcceptSyncRequest struct {
	L       int
	N       Ballot
	Sfx     []interface{}
	SyncIdx int
}

type DecideRequest struct {
	L      int
	N      Ballot
	DecIdx int
}

type AcceptedRequest struct {
	F      int
	N      Ballot
	LogIdx int
}

type AcceptRequest struct {
	L   int
	N   Ballot
	Idx int
	C   interface{}
}

type PrepareReqRequest struct {
	F int
}

type ReconnectedRequest struct {
	F int
}

type OmniPaxosState struct {
	L           Ballot
	Log         []interface{}
	PromisedRnd Ballot
	AcceptedRnd Ballot
	DecidedIdx  int
}

func (r *OmniPaxosState) toBytes() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(r)
	return buf.Bytes(), err
}

func omnipaxosStatefromBytes(b []byte) (OmniPaxosState, error) {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	s := OmniPaxosState{}
	err := dec.Decode(&s)
	return s, err
}

type Ballot struct {
	N   int
	Pid int
}

func (b *Ballot) compare(o Ballot) int {
	if b.N < o.N {
		return -1
	} else if b.N == o.N {
		if b.Pid < o.Pid {
			return -1
		} else if b.Pid == o.Pid {
			return 0
		}
		return 1
	}
	return 1
}

const (
	// role
	LEADER   = "LEADER"
	FOLLOWER = "FOLLOWER"

	// phase
	PREPARE = "PREPARE"
	ACCEPT  = "ACCEPT"
	RECOVER = "RECOVER"
)

type State struct {
	role  string
	phase string
}

func (op *OmniPaxos) HeartBeatHandler(args *HBRequest, reply *HBReply) {
	// Your code here (2A).
	op.mu.Lock()
	defer func() { op.mu.Unlock() }()

	reply.Ballot = op.b.N
	reply.Q = op.qc
	reply.Rnd = args.Rnd
}

// GetState Return the current leader's ballot and whether this server
// believes it is the leader.
func (op *OmniPaxos) GetState() (int, bool) {
	op.mu.Lock()
	defer func() { op.mu.Unlock() }()

	var ballot int
	var isleader bool

	// Your code here (2A).
	ballot = op.b.N
	isleader = (op.state.role == LEADER) && (op.os.L.Pid == op.me)
	op.Debug("returning GetState, ballot:%d, isLeader:%t, state:%+v, rs:%+v", ballot, isleader, op.state, op.os)
	return ballot, isleader
}

// Called by the tester to submit a log to your OmniPaxos server
// Implement this as described in Figure 3
func (op *OmniPaxos) Proposal(command interface{}) (int, int, bool) {
	op.mu.Lock()
	defer func() { op.mu.Unlock() }()

	index := -1
	ballot := -1
	isLeader := (op.state.role == LEADER) && (op.os.L.Pid == op.me)
	op.Debug("started Proposal: %+v, state: %+v", command, op.state)

	// Your code here (2B).
	if op.stopped() {
		return index, ballot, isLeader
	}

	if op.state.role == LEADER && op.state.phase == PREPARE {
		// P1. add to buffer if in prepare
		op.buffer = append(op.buffer, command)
		return len(op.os.Log) - 1, op.os.L.N, isLeader
	} else if op.state.role == LEADER && op.state.phase == ACCEPT {
		// A1. append to log and set accepted
		op.os.Log = append(op.os.Log, command)
		op.accepted[op.me] = len(op.os.Log)
		op.persist()

		// A2. send accept to all promised followers
		var wg sync.WaitGroup
		for _, p := range op.promises {
			if p.f == op.me {
				continue
			}
			wg.Add(1)
			// TODO: figure out if go routines should be used or not
			go func(f int, l int, n Ballot, C interface{}) {
				defer wg.Done()
				request := AcceptRequest{
					L:   l,
					N:   n,
					Idx: len(op.os.Log),
					C:   C,
				}
				op.peers[f].Call("OmniPaxos.AcceptHandler", &request, &DummyReply{})
			}(p.f, op.me, op.currentRnd, command)
		}
		wg.Wait()
		return len(op.os.Log) - 1, op.os.L.N, isLeader
	}

	return index, ballot, isLeader
}

// The service using OmniPaxos (e.g. a k/v server) wants to start
// agreement on the next command to be appended to OmniPaxos's log. If this
// server isn't the leader, returns false. Otherwise start the
// agreement and return immediately. There is no guarantee that this
// command will ever be committed to the OmniPaxos log, since the leader
// may fail or lose an election. Even if the OmniPaxos instance has been killed,
// this function should return gracefully.
//
// The first return value is the index that the command will appear at
// if it's ever committed. The second return value is the current
// ballot. The third return value is true if this server believes it is
// the leader.

// The tester doesn't halt goroutines created by OmniPaxos after each test,
// but it does call the Kill() method. Your code can use killed() to
// check whether Kill() has been called. The use of atomic avoids the
// need for a lock.
//
// The issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. Any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (op *OmniPaxos) Kill() {
	atomic.StoreInt32(&op.dead, 1)
	// Your code here, if desired.
	// you may set a variable to false to
	// disable logs as soon as a server is killed
	atomic.StoreInt32(&op.enableLogging, 0)
}

func (op *OmniPaxos) killed() bool {
	z := atomic.LoadInt32(&op.dead)
	return z == 1
}

func (op *OmniPaxos) max(m map[Ballot]bool) Ballot {
	res := Ballot{N: -1, Pid: -2}
	for b := range m {
		if res.compare(b) < 0 {
			res = b
		}
	}
	return res
}

func (op *OmniPaxos) checkLeader() {
	candidates := map[Ballot]bool{}
	for b, q := range op.ballots {
		if q {
			candidates[b] = true
		}
	}

	max := op.max(candidates)

	L := op.os.L

	op.Trace("inside checkLeader, round:%d, I:n:%d, I:pid:%d, max:n:%d, max:pid:%d, compare:%d, ballots:%+v", op.r, L.N, L.Pid, max.N, max.Pid, max.compare(L), op.ballots)

	if max.compare(L) < 0 {
		op.increment(&op.b, L.N)
		op.qc = true
		op.Trace("setting qc round:%d, max:N:%d, max:pid:%d - previous leader:N:%d, pid:%d, promisedRnd:%d, ballots:%+v, state:%+v", op.r, max.N, max.Pid, L.N, L.Pid, op.os.PromisedRnd, op.ballots, op.state)
	} else if max.compare(L) > 0 {
		op.Info("setting leader for round:%d, max:N:%d, max:pid:%d - previous leader:N:%d, pid:%d, promisedRnd:%d, ballots:%+v, state:%+v", op.r, max.N, max.Pid, L.N, L.Pid, op.os.PromisedRnd, op.ballots, op.state)
		op.os.L = max
		op.persist()
		op.Debug("updated leader L:%+v", op.os.L)
		op.triggerLeader(max.Pid, max)
	}
}

func (op *OmniPaxos) triggerLeader(s int, n Ballot) {
	if !(s == op.me && n.compare(op.os.PromisedRnd) > 0) {
		op.state.role = FOLLOWER
		return
	}
	op.Info("making itself as leader")

	// 1. reset state
	op.accepted = make([]int, len(op.accepted))
	op.promises = map[int]*Promise{}
	op.maxProm = &Promise{}
	op.buffer = []interface{}{}

	// 2.
	op.qc = true
	op.state = State{role: LEADER, phase: PREPARE}
	op.currentRnd = n
	op.os.PromisedRnd = n

	// 3. add own promise
	promise := Promise{f: op.me, accRnd: op.os.AcceptedRnd, logIdx: len(op.os.Log),
		decIdx: op.os.DecidedIdx, log: op.suffix(op.os.DecidedIdx)}
	op.promises[op.me] = &promise

	// 4. send prepare to all peers
	for peer := range op.peers {
		if peer != op.me {
			go func(peer int, pid int, currentRnd Ballot, acceptedRnd Ballot, logSize int, decidedIdx int) {
				request := PrepareRequest{
					L:      pid,
					N:      currentRnd,
					AccRnd: acceptedRnd,
					LogIdx: logSize,
					DecIdx: decidedIdx,
				}
				op.peers[peer].Call("OmniPaxos.PrepareHandler", &request, &DummyReply{})
			}(peer, op.me, op.currentRnd, op.os.AcceptedRnd, len(op.os.Log), op.os.DecidedIdx)
		}
	}

	op.persist()
}

func (op *OmniPaxos) PrepareHandler(req *PrepareRequest, reply *DummyReply) {
	go func(l int, n Ballot, accRnd Ballot, logIdx int, decIdx int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle prepare, l:%d, n:%+v, accRnd:%d, logIdx:%d, decIdx:%d, promRnd:%+v",
			l, n, accRnd, logIdx, decIdx, op.os.PromisedRnd)

		// 1.
		if op.os.PromisedRnd.compare(n) > 0 {
			return
		}

		// 2. update state
		op.state = State{
			role:  FOLLOWER,
			phase: PREPARE,
		}

		// 3. updated promised round
		op.os.PromisedRnd = n
		op.persist()

		// 4. find suffix
		var sfx []interface{}
		if op.os.AcceptedRnd.compare(accRnd) > 0 {
			sfx = op.suffix(decIdx)
		} else if op.os.AcceptedRnd.compare(accRnd) == 0 {
			sfx = op.suffix(logIdx)
		} else {
			sfx = []interface{}{}
		}

		// 5. send promise to leader
		request := PromiseRequest{
			F:      op.me,
			N:      n,
			AccRnd: op.os.AcceptedRnd,
			LogIdx: len(op.os.Log),
			DecIdx: op.os.DecidedIdx,
			Sfx:    sfx,
		}
		op.peers[l].Call("OmniPaxos.PromiseHandler", &request, &DummyReply{})
	}(req.L, req.N, req.AccRnd, req.LogIdx, req.DecIdx)
}

func (op *OmniPaxos) PromiseHandler(req *PromiseRequest, reply *DummyReply) {
	go func(f int, n Ballot, accRnd Ballot, logIdx int, decIdx int, sfx []interface{}) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle Promise, f:%d, n:%+v, accRnd:%d, logIdx:%d, decIdx:%d, sfx:%+v",
			f, n, accRnd, logIdx, decIdx, sfx)

		// 1.
		if n.compare(op.currentRnd) != 0 {
			return
		}

		// 2. update promise
		promise := Promise{
			f:      f,
			accRnd: accRnd,
			logIdx: logIdx,
			decIdx: decIdx,
			log:    sfx,
		}
		op.promises[f] = &promise

		// return if not a leader
		if op.state.role != LEADER {
			return
		}

		if op.state.phase == PREPARE {

			// P1. return if there are no majority yet
			if len(op.promises) < (len(op.peers)+1)/2 {
				return
			}

			// P2. find the maximum promise (with accRnd and logIdx)
			op.maxProm = op.maxPromise()

			// P3. remove extra logs if the leaders accepted round is not same as maximum promises
			if op.maxProm.accRnd != op.os.AcceptedRnd {
				op.os.Log = op.prefix(op.os.DecidedIdx)
			}

			// P4. append max promise's suffix to the log
			op.os.Log = append(op.os.Log, op.maxProm.log...)

			// P5. append buffer to the log unless it is stopped
			if op.stopped() {
				op.buffer = []interface{}{}
			} else {
				op.os.Log = append(op.os.Log, op.buffer...)
			}

			// P6. set accpeted round to current round, updated accepted for leader and set state
			op.os.AcceptedRnd = op.currentRnd
			op.accepted[op.me] = len(op.os.Log)
			op.state = State{role: LEADER, phase: ACCEPT}

			// P5. send AcceptSync to each promised follower
			for _, p := range op.promises {
				var syncIdx int
				if p.accRnd == op.maxProm.accRnd {
					syncIdx = p.logIdx
				} else {
					syncIdx = p.decIdx
				}

				go func(l int, n Ballot, sfx []interface{}, syncIdx int, f int) {
					request := AcceptSyncRequest{
						L:       l,
						N:       n,
						Sfx:     sfx,
						SyncIdx: syncIdx,
					}
					op.peers[f].Call("OmniPaxos.AcceptSyncHandler", &request, &DummyReply{})
				}(op.me, n, op.suffix(syncIdx), syncIdx, p.f)
			}

		} else if op.state.phase == ACCEPT {

			// A1. set syncIdx
			var syncIdx int
			if accRnd == op.maxProm.accRnd {
				syncIdx = op.maxProm.logIdx
			} else {
				syncIdx = decIdx
			}

			// A2. send AcceptSync to follower
			request := AcceptSyncRequest{
				L:       op.me,
				N:       n,
				Sfx:     op.suffix(syncIdx),
				SyncIdx: syncIdx,
			}
			op.peers[f].Call("OmniPaxos.AcceptSyncHandler", &request, &DummyReply{})

			// A3. send decide to follower if they are behind
			if op.os.DecidedIdx > decIdx {
				request := DecideRequest{
					L:      op.me,
					N:      n,
					DecIdx: op.os.DecidedIdx,
				}
				op.peers[f].Call("OmniPaxos.DecideHandler", &request, &DummyReply{})
			}
		}

		op.persist()

	}(req.F, req.N, req.AccRnd, req.LogIdx, req.DecIdx, req.Sfx)
}

func (op *OmniPaxos) AcceptSyncHandler(req *AcceptSyncRequest, reply *DummyReply) {
	go func(l int, n Ballot, sfx []interface{}, syncIdx int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle AcceptSync, l:%d, n:%+v, sfx:%+v, syncIdx:%d, state: %+v, promisedRound:%d",
			l, n, sfx, syncIdx, op.state, op.os.PromisedRnd)

		// 1. only allowed for follower prepare
		if (op.os.PromisedRnd.compare(n) != 0) || !(op.state.role == FOLLOWER && op.state.phase == PREPARE) {
			return
		}

		// 2. update accepted round and state
		op.os.AcceptedRnd = n
		op.state = State{role: FOLLOWER, phase: ACCEPT}

		// 3. update the log
		op.os.Log = op.prefix(syncIdx)
		op.os.Log = append(op.os.Log, sfx...)

		op.persist()

		// 4. send accepted to leader
		request := AcceptedRequest{
			F:      op.me,
			N:      n,
			LogIdx: len(op.os.Log),
		}

		op.peers[l].Call("OmniPaxos.AcceptedHandler", &request, &DummyReply{})
	}(req.L, req.N, req.Sfx, req.SyncIdx)
}

func (op *OmniPaxos) DecideHandler(req *DecideRequest, reply *DummyReply) {
	go func(l int, n Ballot, decIdx int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle Decide, l:%d, n:%+v, decIdx:%d, decidedIdx:%d, state:%+v, log:%+v",
			l, n, decIdx, op.os.DecidedIdx, op.state, op.os.Log)

		// TODO: figure out why it might happen
		if decIdx <= op.os.DecidedIdx {
			return
		}
		if decIdx > len(op.os.Log) {
			return
		}

		// 1. only if it is in follower,accept then update decided index
		if op.os.PromisedRnd.compare(n) == 0 && (op.state.role == FOLLOWER && op.state.phase == ACCEPT) {
			op.addToApplyChan(op.os.Log, op.os.DecidedIdx, decIdx)
			op.os.DecidedIdx = decIdx
			op.persist()
		}

	}(req.L, req.N, req.DecIdx)
}

func (op *OmniPaxos) addToApplyChan(log []interface{}, from int, to int) {
	for i := from; i < to; i++ {
		op.applyCh <- ApplyMsg{
			CommandValid: true,
			Command:      log[i],
			CommandIndex: i,
		}
	}
}

func (op *OmniPaxos) AcceptedHandler(req *AcceptedRequest, reply *DummyReply) {
	go func(f int, n Ballot, logIdx int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle Accepted, f:%d, n:%+v, logIdx:%d, decidedIdx:%d, promisedRnd:%d, state:%+v, accepted:%+v",
			f, n, logIdx, op.os.DecidedIdx, op.os.PromisedRnd, op.state, op.accepted)

		// 1. only accepted for leader,accept
		if op.os.PromisedRnd.compare(n) != 0 || !(op.state.role == LEADER && op.state.phase == ACCEPT) {
			return
		}

		// 2. updated accepted index
		op.accepted[f] = logIdx

		// 3. If follower has bigger log index and majority of followers have accepted it, then update decided index
		// and send decide to all promised followers with the new index
		if logIdx > op.os.DecidedIdx && op.hasMajorityAccepted(logIdx) {
			op.addToApplyChan(op.os.Log, op.os.DecidedIdx, logIdx)
			op.os.DecidedIdx = logIdx
			op.persist()

			for _, p := range op.promises {
				go func(f int, l int, currRnd Ballot, decIdx int) {
					request := DecideRequest{
						L:      l,
						N:      currRnd,
						DecIdx: decIdx,
					}
					op.peers[f].Call("OmniPaxos.DecideHandler", &request, &DummyReply{})
				}(p.f, op.me, op.currentRnd, op.os.DecidedIdx)
			}
		}

	}(req.F, req.N, req.LogIdx)
}

func (op *OmniPaxos) hasMajorityAccepted(v int) bool {
	acceptedCount := 0
	for _, a := range op.accepted {
		if a >= v {
			acceptedCount++
		}
	}
	return acceptedCount >= (len(op.peers)+1)/2
}

func (op *OmniPaxos) AcceptHandler(req *AcceptRequest, reply *DummyReply) {
	go func(l int, n Ballot, idx int, C interface{}) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle Accept, l:%d, n:%+v, C:%d, state: %+v", l, n, C, op.state)

		// 1. only accept if follower,accept
		if op.os.PromisedRnd.compare(n) != 0 || !(op.state.role == FOLLOWER && op.state.phase == ACCEPT) {
			return
		}

		// if idx != len(op.rs.Log) {
		// 	op.reconnect(l)
		// 	return
		// }

		// 2. append to log and send accepted to leader
		op.os.Log = append(op.os.Log, C)
		op.persist()
		go func(l int, f int, n Ballot, logIdx int, C interface{}) {
			request := AcceptedRequest{F: f, N: n, LogIdx: logIdx}
			op.peers[l].Call("OmniPaxos.AcceptedHandler", &request, &DummyReply{})
		}(l, op.me, n, len(op.os.Log), C)

	}(req.L, req.N, req.Idx, req.C)
}

func (op *OmniPaxos) ReconnectedHandler(req *ReconnectedRequest, reply *DummyReply) {
	go func(f int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle Reconnected, f:%d", f)

		if f == op.os.L.Pid {
			op.state = State{role: FOLLOWER, phase: RECOVER}
		}

		request := PrepareReqRequest{F: op.me}
		op.peers[f].Call("OmniPaxos.PrepareReqHandler", &request, &DummyReply{})

	}(req.F)
}

func (op *OmniPaxos) PrepareReqHandler(req *PrepareReqRequest, reply *DummyReply) {
	go func(f int) {
		op.mu.Lock()
		defer func() { op.mu.Unlock() }()

		op.Debug("Handle PrepareReq, f:%d", f)

		if op.state.role != LEADER {
			return
		}
		request := PrepareRequest{
			L:      op.me,
			N:      op.currentRnd,
			AccRnd: op.os.AcceptedRnd,
			LogIdx: len(op.os.Log),
			DecIdx: op.os.DecidedIdx,
		}
		op.peers[f].Call("OmniPaxos.PrepareHandler", &request, &DummyReply{})
	}(req.F)
}

func (op *OmniPaxos) stopped() bool {
	// TODO: fixme
	return false
}

func (op *OmniPaxos) maxPromise() *Promise {
	max := &Promise{accRnd: Ballot{N: -1, Pid: -1}, log: []interface{}{}}
	for _, p := range op.promises {
		if (p.accRnd.compare(max.accRnd) > 0) || (p.accRnd == max.accRnd && p.logIdx > max.logIdx) {
			max = p
		}
	}
	return max
}

func (op *OmniPaxos) suffix(idx int) []interface{} {
	if idx > len(op.os.Log) {
		return []interface{}{}
	}
	return op.os.Log[idx:]
}

func (op *OmniPaxos) prefix(idx int) []interface{} {
	if idx > len(op.os.Log) {
		idx = len(op.os.Log)
	}
	return op.os.Log[:idx]
}

func (op *OmniPaxos) increment(ballot *Ballot, I int) {
	ballot.N = I + 1
}

func (op *OmniPaxos) startTimer(delay time.Duration) {
	for {
		op.mu.Lock()

		// 1. insert own ballot
		op.ballots[op.b] = op.qc

		// 2. if have majority, then check leader else qc is false
		if len(op.ballots) >= (len(op.peers)+1)/2 {
			op.checkLeader()
		} else {
			op.qc = false
			// op.state = State{role: FOLLOWER, phase: PREPARE}
		}

		// add to missed heartbeat count to keep track of reconnections
		op.updateMissedHbsAndReconnect()

		// 3. clear ballot and increase the round
		op.ballots = make(map[Ballot]bool)
		op.r++
		op.mu.Unlock()

		// 4. send heartbeat to all peers
		for peer := range op.peers {
			if peer != op.me {
				go func(r int, p int) {
					request := HBRequest{Rnd: r}
					reply := HBReply{}
					ok := op.peers[p].Call("OmniPaxos.HeartBeatHandler", &request, &reply)
					op.Trace("received heart beat from %d, round:%d, ballot:%d, q:%t", p, reply.Rnd, reply.Ballot, reply.Q)

					if ok && reply.Rnd == r {
						op.mu.Lock()
						op.ballots[Ballot{N: reply.Ballot, Pid: p}] = reply.Q
						op.mu.Unlock()
					}
				}(op.r, peer)
			}
		}

		time.Sleep(delay)
	}

}

func (op *OmniPaxos) updateMissedHbsAndReconnect() {
	allMissed := map[int]bool{}
	for peer := range op.peers {
		if peer != op.me {
			allMissed[peer] = true
		}
	}

	op.Trace("missed hb counts:%+v, ballots:%+v, leader:%+v, started:%t", op.missedHbCounts, op.ballots, op.os.L)

	for b := range op.ballots {
		if b.Pid == op.me {
			continue
		}

		isLinkReconnected := op.missedHbCounts[b.Pid] > 3 && op.me == op.os.L.Pid
		if isLinkReconnected {
			request := ReconnectedRequest{F: op.me}
			op.peers[b.Pid].Call("OmniPaxos.ReconnectedHandler", &request, &DummyReply{})
		}
		op.missedHbCounts[b.Pid] = 0
		delete(allMissed, b.Pid)
	}

	for p := range allMissed {
		op.missedHbCounts[p]++
	}
	if len(allMissed) == len(op.peers)-1 {
		if !op.linkDrop {
			op.Info("link dropped")
		}
		op.linkDrop = true
	} else if op.linkDrop {
		op.Info("link recovered")
		op.linkDrop = false
	}

	op.restart.loop++
	if op.restart.loop == 2 {
		for p := range op.peers {
			request := PrepareReqRequest{F: op.me}
			op.peers[p].Call("OmniPaxos.PrepareReqHandler", &request, &DummyReply{})
		}
	}

}

// save OmniPaxos's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 3 &4 for a description of what should be persistent.
func (op *OmniPaxos) persist() {
	buf, _ := op.os.toBytes()
	op.persister.SaveOmnipaxosState(buf)
}

// restore previously persisted state.
func (op *OmniPaxos) readPersist() {
	op.os, _ = omnipaxosStatefromBytes(op.persister.ReadOmnipaxosState())
}

func (op *OmniPaxos) checkRecover() {
	op.mu.Lock()
	if op.os.PromisedRnd.Pid != -1 {
		op.Trace("recovering the server")
		op.state = State{role: FOLLOWER, phase: RECOVER}
		for p := range op.peers {
			go func(p int, me int) {
				request := PrepareReqRequest{F: me}
				op.peers[p].Call("OmniPaxos.PrepareReqHandler", &request, &DummyReply{})
			}(p, op.me)

		}
	}
	op.mu.Unlock()
}

// The service or tester wants to create a OmniPaxos server. The ports
// of all the OmniPaxos servers (including this one) are in peers[]. This
// server's port is peers[me]. All the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects OmniPaxos to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *OmniPaxos {

	op := &OmniPaxos{}
	op.peers = peers
	op.persister = persister
	op.me = me
	op.applyCh = applyCh

	op.enableLogging = 0
	op.Info("initializing Omni Paxos instance")

	// initialize variables
	op.delay = time.Millisecond * 100
	op.qc = true
	op.r = 0
	op.b = Ballot{N: 0, Pid: me}
	op.ballots = make(map[Ballot]bool)
	op.accepted = make([]int, len(peers))
	op.promises = map[int]*Promise{}
	op.buffer = []interface{}{}

	op.state = State{role: FOLLOWER, phase: PREPARE}
	op.os = OmniPaxosState{L: Ballot{N: -1, Pid: -1}, Log: []interface{}{}, PromisedRnd: Ballot{N: -1, Pid: -1}, AcceptedRnd: Ballot{N: -1, Pid: -1}, DecidedIdx: 0}
	op.missedHbCounts = map[int]int{}
	op.restart = Restart{}

	op.readPersist()
	op.addToApplyChan(op.os.Log, 0, op.os.DecidedIdx)

	go op.startTimer(op.delay)

	go func() {
		time.Sleep(op.delay)
		// op.checkRecover()
	}()

	return op
}
