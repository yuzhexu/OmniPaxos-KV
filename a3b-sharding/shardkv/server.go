package shardkv

import (
	omnipaxos "cs651/a2-omnipaxos"
	"cs651/a3b-sharding/shardctrler"
	"cs651/labgob"
	"cs651/labrpc"
	"sync"
	"time"
)

const (
	CGET    = "get"
	CPUT    = "put"
	CAPPEND = "append"
	CCHANGE = "change"
	CDELETE = "delete"
)

const NShards = 10
const delay = 250 * time.Millisecond

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.

	// common
	Command   string
	ClientId  int64
	RequestId int64

	// get and put
	Key   string
	Value string

	// update config
	Config  shardctrler.Config
	Store   [NShards]map[string]string
	PrevReq map[int64]int64

	// delete shards.
	ConfigNum int
	ShardId   int
}

type Result struct {
	// common
	Command string
	Err     Err

	// common
	ClientId  int64
	RequestId int64

	// get
	Value string

	// configuration.
	ConfigNum int
}

type ShardKV struct {
	mu                sync.Mutex
	me                int
	op                *omnipaxos.OmniPaxos
	applyCh           chan omnipaxos.ApplyMsg
	make_end          func(string) *labrpc.ClientEnd
	gid               int
	masters           []*labrpc.ClientEnd
	maxOmniPaxosstate int

	// Your definitions here.
	// config
	config shardctrler.Config

	// store
	store [NShards]map[string]string

	// complted requests
	prevReq map[int64]int64

	// notify
	chans map[int]chan Result

	// clerk to query
	clerk *shardctrler.Clerk
}

func (kv *ShardKV) propose(op Op) Result {
	// propose to omnipaxos
	ind, _, isLeader := kv.op.Proposal(op)
	if !isLeader {
		return Result{Err: ErrNotLeader}
	}

	// create channel
	kv.mu.Lock()
	ch, ok := kv.chans[ind]
	if !ok {
		ch = make(chan Result, 1)
		kv.chans[ind] = ch
	}
	kv.mu.Unlock()

	// wait for result and return if it is expected
	select {
	case result := <-ch:
		switch op.Command {
		case CCHANGE:
			if op.Config.Num == result.ConfigNum {
				return result
			}
		case CDELETE:
			if op.ConfigNum == result.ConfigNum {
				return result
			}
		default:
			if op.ClientId == result.ClientId && op.RequestId == result.RequestId {
				return result
			}
		}

		return Result{Err: ErrTryAgain}
	case <-time.After(delay):
		// timeout
		return Result{Err: ErrTimeout}
	}
}

func (kv *ShardKV) Get(args *GetArgs, reply *GetReply) {
	// Your code here.
	op := Op{Command: CGET, ClientId: args.ClientId, RequestId: args.RequestId, Key: args.Key}
	result := kv.propose(op)
	reply.Err = result.Err
	reply.Value = result.Value
}

func (kv *ShardKV) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	op := Op{Command: args.Op, ClientId: args.ClientId, RequestId: args.RequestId, Key: args.Key, Value: args.Value}
	result := kv.propose(op)
	reply.Err = result.Err
}

func (kv *ShardKV) Transfer(args *TransferArgs, reply *TransferReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	if kv.config.Num < args.Num {
		// not upto date
		reply.Err = ErrTryAgain
		return
	}

	// copy the store and prev completely and return in reply
	reply.Prev = make(map[int64]int64)
	copyMapInt(kv.prevReq, reply.Prev)

	for i := 0; i < NShards; i++ {
		reply.Store[i] = make(map[string]string)
		copyMapStr(kv.store[i], reply.Store[i])
	}

	reply.Err = OK
}

func copyMapInt(src, dst map[int64]int64) {
	for k, v := range src {
		dst[k] = v
	}
}

func copyMapStr(src, dst map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func (kv *ShardKV) Delete(args *DeleteArgs, reply *DeleteReply) {
	_, isLeader := kv.op.GetState()
	if !isLeader {
		reply.Err = ErrNotLeader
		return
	}

	if args.Num > kv.config.Num {
		reply.Err = ErrTryAgain
		return
	}

	op := Op{Command: CDELETE, ConfigNum: args.Num, ShardId: args.ShardId}
	kv.propose(op)
	reply.Err = OK
}

func emptyStore() [NShards]map[string]string {
	var m [NShards]map[string]string
	for i := 0; i < NShards; i++ {
		m[i] = make(map[string]string)
	}
	return m
}

// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
func (kv *ShardKV) Kill() {
	kv.op.Kill()
	// Your code here, if desired.
}

func StartServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister, maxOmniPaxosstate int, gid int, masters []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})
	labgob.Register(Result{})
	labgob.Register(shardctrler.Config{})

	kv := new(ShardKV)
	kv.me = me
	kv.maxOmniPaxosstate = maxOmniPaxosstate
	kv.make_end = make_end
	kv.gid = gid
	kv.masters = masters

	// Your initialization code here.
	kv.applyCh = make(chan omnipaxos.ApplyMsg, 100)
	kv.op = omnipaxos.Make(servers, me, persister, kv.applyCh)

	kv.store = emptyStore()
	kv.prevReq = make(map[int64]int64)
	kv.chans = make(map[int]chan Result)
	kv.clerk = shardctrler.MakeClerk(masters)

	go kv.applier()
	go kv.catchUp()

	return kv
}
