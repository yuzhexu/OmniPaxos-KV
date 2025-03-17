package shardctrler

//
// Shardctrler clerk.
//

import (
	"crypto/rand"
	"cs651/labrpc"
	"math/big"
	"time"
)

const reqDelay = 100 * time.Millisecond

type Clerk struct {
	servers []*labrpc.ClientEnd
	// Your data here.
	clientId  int64
	requestId int64
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// Your code here.
	bigx, _ := rand.Int(rand.Reader, big.NewInt(int64(1)<<62))
	ck.clientId = bigx.Int64()
	ck.requestId = 1
	return ck
}

func (ck *Clerk) Query(num int) Config {
	args := &QueryArgs{ClientId: ck.clientId, RequestId: ck.requestId, ConfNum: num}
	// Your code here.
	for {
		for _, s := range ck.servers {
			reply := QueryReply{}
			if ok := s.Call("ShardCtrler.Query", args, &reply); ok && reply.Err == "" {
				ck.requestId++
				return reply.Config
			}
		}
		time.Sleep(reqDelay)
	}
}

func (ck *Clerk) Join(servers map[int][]string) {
	args := &JoinArgs{ClientId: ck.clientId, RequestId: ck.requestId, Servers: servers}
	// Your code here.
	for {
		for _, s := range ck.servers {
			reply := JoinReply{}
			if ok := s.Call("ShardCtrler.Join", args, &reply); ok && reply.Err == "" {
				ck.requestId++
				return
			}
		}
		time.Sleep(reqDelay)
	}
}

func (ck *Clerk) Leave(gids []int) {
	args := &LeaveArgs{ClientId: ck.clientId, RequestId: ck.requestId, GIDs: gids}
	// Your code here.
	for {
		for _, s := range ck.servers {
			reply := LeaveReply{}
			if ok := s.Call("ShardCtrler.Leave", args, &reply); ok && reply.Err == "" {
				ck.requestId++
				return
			}
		}
		time.Sleep(reqDelay)
	}
}

func (ck *Clerk) Move(shard int, gid int) {
	args := &MoveArgs{ClientId: ck.clientId, RequestId: ck.requestId, Shard: shard, GID: gid}
	// Your code here.
	for {
		for _, s := range ck.servers {
			reply := MoveReply{}
			if ok := s.Call("ShardCtrler.Move", args, &reply); ok && reply.Err == "" {
				ck.requestId++
				return
			}
		}
		time.Sleep(reqDelay)
	}
}
