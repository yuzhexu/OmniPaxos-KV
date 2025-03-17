package kvpaxos

import (
	"encoding/gob"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	omnipaxos "cs651/a2-omnipaxos"
	"cs651/labgob"
	"cs651/labrpc"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type KVCommand struct {
	Id      int
	Command interface{}
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
}

type Response struct {
	command interface{}
	value   string
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *omnipaxos.OmniPaxos
	applyCh chan omnipaxos.ApplyMsg
	dead    int32 // set by Kill()

	enableLogging int32

	// Your definitions here.
	response []Response
	seq      map[int]int
	m        map[string]string
}

const waitTime = 50 * time.Millisecond
const NOT_LEADER = "not a leader"

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	// Your code here.

	index, _, isLeader := kv.rf.Proposal(KVCommand{Id: kv.me, Command: *args})
	if index == -1 || !isLeader {
		kv.Debug("not a leader, received get key:%s", args.Key)
		reply.Err = NOT_LEADER
		return
	}

	kv.Info("is leader at index:%d, received get key:%s, id:%s", index, args.Key, args.Id)

	count := 0
	for {
		kv.mu.Lock()
		kv.Trace("looping for message:%+v, index:%d, loglen:%d", *args, index, len(kv.response))
		if len(kv.response) > index {
			r := kv.response[index]
			kv.mu.Unlock()

			kvc := r.command.(KVCommand)
			c, ok := kvc.Command.(GetArgs)
			if !ok || c.Key != (*args).Key {
				reply.Err = "try again"
				return
			}
			reply.Value = r.value
			return
		}
		kv.mu.Unlock()
		time.Sleep(waitTime)
		count++
		if count > 5 {
			reply.Err = "timeout"
			return
		}
	}
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	// Your code here.
	index, _, isLeader := kv.rf.Proposal(KVCommand{Id: kv.me, Command: *args})
	if index == -1 || !isLeader {
		kv.Debug("not a leader, received put/append key:%s, op:%s, value:%s, id:%s", args.Key, args.Op, args.Value, args.Id)
		reply.Err = NOT_LEADER
		return
	}

	kv.Info("is leader at index:%d, received put/append key:%s, op:%s, value:%s, id:%s", index, args.Key, args.Op, args.Value, args.Id)

	count := 0
	for {
		kv.mu.Lock()
		kv.Debug("looping for message:%+v, index:%d, loglen:%d", *args, index, len(kv.response))

		if len(kv.response) > index {
			r := kv.response[index]
			kv.mu.Unlock()

			kvc := r.command.(KVCommand)
			c, ok := kvc.Command.(PutAppendArgs)
			if !ok || c.Key != (*args).Key || c.Value != (*args).Value || c.Op != (*args).Op {
				reply.Err = "try again"
				return
			}
			return
		}
		kv.mu.Unlock()
		time.Sleep(waitTime)
		count++
		if count > 5 {
			reply.Err = "timeout"
			return
		}
	}

}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// servers[] contains the ports of the set of
// servers that will cooperate via OmniPaxos to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying OmniPaxos
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the OmniPaxos state along with the snapshot.
// the k/v server should snapshot when OmniPaxos's saved state exceeds maxomnipaxosstate bytes,
// in order to allow OmniPaxos to garbage-collect its log. if maxomnipaxosstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *omnipaxos.Persister, maxomnipaxosstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})
	gob.Register(GetArgs{})
	gob.Register(GetReply{})
	gob.Register(PutAppendArgs{})
	gob.Register(PutAppendReply{})
	gob.Register(KVCommand{})

	kv := new(KVServer)
	kv.me = me
	// to enable logging
	// set to 0 to disable
	kv.enableLogging = 1

	// You may need initialization code here.
	kv.applyCh = make(chan omnipaxos.ApplyMsg, 1000)
	kv.rf = omnipaxos.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	kv.response = []Response{}
	kv.m = make(map[string]string)
	kv.seq = make(map[int]int)
	go kv.readApply()
	return kv
}

func (kv *KVServer) readApply() {
	for {
		msg := <-kv.applyCh
		kv.Info("applying message %+v", msg)
		kvc := msg.Command.(KVCommand)
		switch v := kvc.Command.(type) {
		case GetArgs:
			arg := kvc.Command.(GetArgs)
			cid, seqNo := parseId(arg.Id)
			kv.mu.Lock()
			if kv.seq[cid] < seqNo {
				kv.seq[cid] = seqNo
			}
			kv.response = append(kv.response, Response{command: msg.Command, value: kv.m[arg.Key]})
			kv.mu.Unlock()
		case PutAppendArgs:
			arg := kvc.Command.(PutAppendArgs)
			kv.mu.Lock()
			cid, seqNo := parseId(arg.Id)
			kv.Debug("applying putappendargs message %+v, %+v", msg, kv.seq)
			if kv.seq[cid] < seqNo {
				if arg.Op == "Put" {
					kv.m[arg.Key] = arg.Value
				} else {
					kv.m[arg.Key] = kv.m[arg.Key] + arg.Value
				}
				kv.seq[cid] = seqNo
			}
			kv.response = append(kv.response, Response{command: msg.Command, value: ""})
			kv.mu.Unlock()
			kv.Trace("%+v", v)
		}
	}
}

func parseId(s string) (int, int) {
	a := strings.Split(s, `#`)
	cid, _ := strconv.Atoi(a[0])
	seqNo, _ := strconv.Atoi(a[1])
	return cid, seqNo
}
