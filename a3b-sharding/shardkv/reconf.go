package shardkv

import (
	"cs651/a3b-sharding/shardctrler"
	"sync"
	"time"
)

const catchUpTime = 100 * time.Millisecond

// catch up with the master configs periodically if it is the leader
func (kv *ShardKV) catchUp() {
	for {
		_, isLeader := kv.op.GetState()
		if !isLeader {
			// try again if not leader
			time.Sleep(catchUpTime)
			continue
		}

		// get latest config
		latest := kv.clerk.Query(shardctrler.LatestConfigNum)
		for i := 1; i <= latest.Num-kv.config.Num; i++ {
			// get config
			conf := kv.clerk.Query(i + kv.config.Num)

			// get the chang operation
			op, ok := kv.getChangeOp(conf)
			if !ok {
				break
			}

			// propose it but break on error
			if res := kv.propose(op); res.Err != OK {
				break
			}
		}

		time.Sleep(catchUpTime)
	}
}

func (kv *ShardKV) getChangeOp(conf shardctrler.Config) (Op, bool) {
	op := Op{Command: CCHANGE, Config: conf, PrevReq: make(map[int64]int64), Store: emptyStore()}

	ok := true
	var wg sync.WaitGroup

	// get shards to transfer
	all := kv.shardsToTransfer(conf.Shards)
	for gid, shardIds := range all {
		wg.Add(1)

		go func(gid, confNum int, shardIds []int) {
			defer wg.Done()

			args := TransferArgs{Num: conf.Num, ShardIds: shardIds}
			reply := TransferReply{}

			// send transfer to all servers in the group
			sent := false
			for _, server := range kv.config.Groups[gid] {
				tok := kv.make_end(server).Call("ShardKV.Transfer", &args, &reply)
				if tok && reply.Err == OK {
					sent = true
					break
				}
			}

			kv.mu.Lock()
			if !sent {
				ok = false
			}

			// if successfully transferred, then use the transferred data
			for _, shardId := range shardIds {
				copyMapStr(reply.Store[shardId], op.Store[shardId])
			}

			for clientId := range reply.Prev {
				op.PrevReq[clientId] = max(op.PrevReq[clientId], reply.Prev[clientId])
			}

			kv.mu.Unlock()

		}(gid, conf.Num, shardIds)
	}
	wg.Wait()
	return op, ok
}

func (kv *ShardKV) shardsToTransfer(shards [NShards]int) map[int][]int {
	m := make(map[int][]int)
	for i := 0; i < NShards; i++ {
		// only other shards
		if kv.config.Shards[i] == kv.gid || shards[i] != kv.gid {
			continue
		}

		gid := kv.config.Shards[i]
		if gid == 0 {
			continue
		}

		if _, ok := m[gid]; !ok {
			m[gid] = []int{}
		}
		m[gid] = append(m[gid], i)

	}
	return m
}
