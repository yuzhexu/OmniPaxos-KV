package shardctrler

import (
	omnipaxos "cs651/a2-omnipaxos"
	"cs651/labgob"
	"cs651/labrpc"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ErrNotLeader = "ErrNotLeader"
	ErrTimeOut   = "ErrTimeOut"
)

const delay = 500 * time.Millisecond
const LatestConfigNum = -1

type ControllerCommand struct {
	// Your data here.
	Command  string
	ReqId    int64
	ClientId int64

	// join
	Servers map[int][]string

	// leave
	GIDs []int

	// query
	Num int

	// move
	Shard int
	GID   int
}

type ShardCtrler struct {
	mu      sync.Mutex
	me      int
	op      *omnipaxos.OmniPaxos
	applyCh chan omnipaxos.ApplyMsg
	dead    int32

	// Your data here.
	curNum   int                 // current config number
	allConfs []Config            // list of all configs
	chans    map[int]chan Result // all clerk response channels
	prevReq  map[int64]int64     // previous requests from clerks
}

func (sc *ShardCtrler) Join(args *JoinArgs, reply *JoinReply) {
	// Your code here.
	result := sc.propose(ControllerCommand{
		Command:  Join,
		ClientId: args.ClientId,
		ReqId:    args.RequestId,
		Servers:  args.Servers,
	})
	reply.Err = result.Err
}

func (sc *ShardCtrler) Leave(args *LeaveArgs, reply *LeaveReply) {
	// Your code here.
	result := sc.propose(ControllerCommand{
		Command:  Leave,
		ClientId: args.ClientId,
		ReqId:    args.RequestId,
		GIDs:     args.GIDs,
	})
	reply.Err = result.Err
}

func (sc *ShardCtrler) Move(args *MoveArgs, reply *MoveReply) {
	// Your code here.
	result := sc.propose(ControllerCommand{
		Command:  Move,
		ClientId: args.ClientId,
		ReqId:    args.RequestId,
		GID:      args.GID,
		Shard:    args.Shard,
	})
	reply.Err = result.Err
}

func (sc *ShardCtrler) Query(args *QueryArgs, reply *QueryReply) {
	// Your code here.
	result := sc.propose(ControllerCommand{
		Command:  Query,
		ClientId: args.ClientId,
		ReqId:    args.RequestId,
		Num:      args.ConfNum,
	})
	reply.Config = result.Config
	reply.Err = result.Err
}

func (sc *ShardCtrler) Kill() {
	sc.op.Kill()
	atomic.StoreInt32(&sc.dead, 1)
	// Your code here, if desired.
}

func (sc *ShardCtrler) killed() bool {
	return atomic.LoadInt32(&sc.dead) == 1
}

// needed by shardkv tester
func (sc *ShardCtrler) OmniPaxos() *omnipaxos.OmniPaxos {
	return sc.op
}

func (sc *ShardCtrler) propose(com ControllerCommand) Result {
	// ignore requests which are old
	sc.mu.Lock()
	if prev, ok := sc.prevReq[com.ClientId]; ok && prev >= com.ReqId {
		sc.mu.Unlock()
		return Result{}
	}
	sc.mu.Unlock()

	// propose and ignore if not leader
	i, _, l := sc.op.Proposal(com)
	if !l {
		return Result{Err: ErrNotLeader}
	}

	// make return chan
	sc.mu.Lock()
	ch := make(chan Result)
	sc.chans[i] = ch
	sc.mu.Unlock()

	// wait for result or timeout
	var res Result
	select {
	case res = <-ch:
		if res.ClientId != com.ClientId || res.RequestId != com.ReqId {
			res.Err = ErrNotLeader
		}
	case <-time.After(delay):
		res.Err = ErrTimeOut
	}

	// remove chan
	sc.mu.Lock()
	delete(sc.chans, i)
	sc.mu.Unlock()

	return res
}

func (sc *ShardCtrler) getConf(num int, confs []Config) Config {
	c := Config{}
	c.Num = num
	c.Shards = confs[num].Shards
	c.Groups = make(map[int][]string)

	c.Num++
	sc.curNum++

	for k, v := range confs[num].Groups {
		s := []string{}
		for _, x := range v {
			s = append(s, x)
		}
		c.Groups[k] = s
	}

	return c
}

func (sc *ShardCtrler) maxGroup(all map[int][]int) int {
	if _, ok := all[0]; ok && len(all[0]) > 0 {
		return 0
	}

	keys := sc.sortedKeys(all)
	max := 0
	gid := 0

	for _, k := range keys {
		if max < len(all[k]) {
			max = len(all[k])
			gid = k
		}
	}
	return gid
}

func (*ShardCtrler) sortedKeys(all map[int][]int) []int {
	keys := make([]int, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func (sc *ShardCtrler) minGroup(all map[int][]int) int {
	min := math.MaxUint32
	gid := 0
	keys := sc.sortedKeys(all)

	for _, k := range keys {
		if min > len(all[k]) && k != 0 {
			min = len(all[k])
			gid = k
		}
	}
	return gid
}

func (sc *ShardCtrler) applyJoin(op ControllerCommand) Result {
	// get conf
	c := sc.getConf(sc.curNum, sc.allConfs)

	// add new ones to all
	all := make(map[int][]int)
	for gid := range op.Servers {
		all[gid] = []int{}
		c.Groups[gid] = op.Servers[gid]
	}

	// add to existing ones to all
	for i := 0; i < NShards; i++ {
		all[c.Shards[i]] = append(all[c.Shards[i]], i)
	}

	// rebalance till there is only 1 distance from min and max
	sc.balance(all, &c, 1)

	sc.allConfs = append(sc.allConfs, c)
	return Result{ClientId: op.ClientId, RequestId: op.ReqId, Config: sc.allConfs[sc.curNum]}
}

func (sc *ShardCtrler) balance(all map[int][]int, c *Config, d int) {
	max, min := sc.maxGroup(all), sc.minGroup(all)
	if max != 0 && len(all[max]) <= d+len(all[min]) {
		return
	}
	first := all[max][0]
	c.Shards[first] = min
	all[max] = all[max][1:]
	all[min] = append(all[min], first)

	sc.balance(all, c, d)
}

func (sc *ShardCtrler) applyLeave(op ControllerCommand) Result {
	// get conf
	c := sc.getConf(sc.curNum, sc.allConfs)

	// remove all gids and add remaining
	all := sc.remove(op, &c)

	// assign the removed ones to min group one by one
	sc.assingToMin(&op, &c, all)

	sc.allConfs = append(sc.allConfs, c)
	return Result{ClientId: op.ClientId, RequestId: op.ReqId, Config: sc.allConfs[sc.curNum]}
}

func (*ShardCtrler) remove(op ControllerCommand, c *Config) map[int][]int {
	for _, gid := range op.GIDs {
		delete(c.Groups, gid)
	}

	all := make(map[int][]int)
	for gid := range c.Groups {
		all[gid] = []int{}
		for i := 0; i < NShards; i++ {
			if gid == c.Shards[i] {
				all[gid] = append(all[gid], i)
			}
		}
	}
	return all
}

func (sc *ShardCtrler) assingToMin(op *ControllerCommand, c *Config, all map[int][]int) {
	for i := 0; i < NShards; i++ {
		for _, gid := range op.GIDs {
			if gid == c.Shards[i] {
				min := sc.minGroup(all)
				c.Shards[i] = min
				if min != 0 {
					all[min] = append(all[min], i)
				}
			}
		}
	}
}

func (sc *ShardCtrler) applyMove(op ControllerCommand) Result {
	c := sc.getConf(sc.curNum, sc.allConfs)
	c.Shards[op.Shard] = op.GID
	sc.allConfs = append(sc.allConfs, c)
	return Result{ClientId: op.ClientId, RequestId: op.ReqId, Config: sc.allConfs[sc.curNum]}
}

func (sc *ShardCtrler) applyQuery(op ControllerCommand) Result {
	num := op.Num
	if op.Num == LatestConfigNum || op.Num > sc.curNum {
		num = sc.curNum
	}
	return Result{ClientId: op.ClientId, RequestId: op.ReqId, Config: sc.allConfs[num]}
}

func (sc *ShardCtrler) applyCommands() {
	for {
		// get message
		m := <-sc.applyCh

		sc.mu.Lock()
		c := m.Command.(ControllerCommand)

		// check if its on old request
		var res Result
		if sc.prevReq[c.ClientId] < c.ReqId {
			sc.prevReq[c.ClientId] = c.ReqId
			// apply command
			switch c.Command {
			case Leave:
				res = sc.applyLeave(c)
			case Join:
				res = sc.applyJoin(c)
			case Query:
				res = sc.applyQuery(c)
			case Move:
				res = sc.applyMove(c)
			default:
				return
			}
		} else {
			// empty response if old request
			res = Result{RequestId: c.ReqId, ClientId: c.ClientId}
		}

		// add to chan
		if n, ok := sc.chans[m.CommandIndex]; ok {
			n <- res
		}
		sc.mu.Unlock()
	}
}

func StartServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister) *ShardCtrler {
	sc := new(ShardCtrler)
	sc.me = me
	sc.dead = 0
	sc.allConfs = make([]Config, 1)
	sc.allConfs[0].Groups = map[int][]string{}
	sc.applyCh = make(chan omnipaxos.ApplyMsg)
	sc.op = omnipaxos.Make(servers, me, persister, sc.applyCh)

	// Your code here.
	labgob.Register(ControllerCommand{})
	sc.prevReq = make(map[int64]int64)
	sc.chans = make(map[int]chan Result)

	go func() { sc.applyCommands() }()
	return sc
}
