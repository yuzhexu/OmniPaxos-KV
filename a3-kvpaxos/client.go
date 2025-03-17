package kvpaxos

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"time"

	"cs651/labrpc"
)

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.
	me  int64
	seq int64
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.
	ck.me = nrand()
	ck.seq = 0
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	args := GetArgs{Key: key, Id: ck.getId()}
	for {
		for _, s := range ck.servers {
			reply := GetReply{}
			ok := s.Call("KVServer.Get", &args, &reply)
			if ok && reply.Err == "" {
				return reply.Value
			}
		}
		time.Sleep(waitTime)
	}
}

func (ck *Clerk) getId() string {
	ck.seq++
	return strconv.Itoa(int(ck.me)) + "#" + strconv.Itoa(int(ck.seq))
}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	// You will have to modify this function.
	args := PutAppendArgs{Key: key, Value: value, Op: op, Id: ck.getId()}
	for {
		for _, s := range ck.servers {
			reply := PutAppendReply{}
			ok := s.Call("KVServer.PutAppend", &args, &reply)
			if ok && reply.Err == "" {
				return
			}
		}
		time.Sleep(waitTime)
	}

}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
