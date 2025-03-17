package shardkv

import "cs651/a3b-sharding/shardctrler"

// check if the request is new or not
func (kv *ShardKV) isNew(op Op) bool {
	p, ok := kv.prevReq[op.ClientId]
	if ok {
		return op.RequestId > p
	}
	return true
}

// check if the key belongs to the group
func (kv *ShardKV) isWrongGroup(key string) bool {
	return kv.config.Shards[key2shard(key)] != kv.gid
}

// get the shard kv for the key
func (kv *ShardKV) shardOf(key string) map[string]string {
	return kv.store[key2shard(key)]
}

// set prev req
func (kv *ShardKV) setPrevReqId(args Op) {
	kv.prevReq[args.ClientId] = args.RequestId
}

func (kv *ShardKV) applyPut(op Op, result *Result) {
	if kv.isWrongGroup(op.Key) {
		result.Err = ErrWrongGroup
		return
	}

	result.Err = OK
	if !kv.isNew(op) {
		return
	}
	kv.shardOf(op.Key)[op.Key] = op.Value
	kv.setPrevReqId(op)
}

func (kv *ShardKV) applyAppend(op Op, result *Result) {
	if kv.isWrongGroup(op.Key) {
		result.Err = ErrWrongGroup
		return
	}

	result.Err = OK
	if !kv.isNew(op) {
		return
	}
	kv.shardOf(op.Key)[op.Key] += op.Value
	kv.setPrevReqId(op)
}

func (kv *ShardKV) applyGet(op Op, result *Result) {
	if kv.isWrongGroup(op.Key) {
		result.Err = ErrWrongGroup
		return
	}
	if kv.isNew(op) {
		kv.setPrevReqId(op)
	}

	v, ok := kv.shardOf(op.Key)[op.Key]
	if !ok {
		result.Err = ErrNoKey
		return
	}

	result.Value = v
	result.Err = OK
}

func (kv *ShardKV) applyChange(op Op, result *Result) {
	result.ConfigNum = op.Config.Num
	result.Err = OK

	if op.Config.Num-kv.config.Num != 1 {
		return
	}

	// merge prev req
	for clientId := range op.PrevReq {
		kv.prevReq[clientId] = max(kv.prevReq[clientId], op.PrevReq[clientId])
	}

	// copy data and send delete request.
	for sid, sd := range op.Store {
		copyMapStr(sd, kv.store[sid])
		if len(sd) > 0 {
			go func(gid, sid int, conf *shardctrler.Config) {
				args := DeleteArgs{Num: kv.config.Num, ShardId: sid}
				reply := DeleteReply{}

				// send delete to all servers
				for _, server := range conf.Groups[gid] {
					if ok := kv.make_end(server).Call("ShardKV.Delete", &args, &reply); ok {
						return
					}
				}
			}(kv.config.Shards[sid], sid, &kv.config)
		}
	}

	kv.config = op.Config
}

func (kv *ShardKV) applyDelete(args Op) {
	if args.ConfigNum <= kv.config.Num && kv.gid != kv.config.Shards[args.ShardId] {
		kv.store[args.ShardId] = make(map[string]string)
	}
}

func (kv *ShardKV) applier() {
	for {
		// get command
		com := <-kv.applyCh

		kv.mu.Lock()
		op := com.Command.(Op)
		res := Result{Command: op.Command, Err: OK, ClientId: op.ClientId, RequestId: op.RequestId}

		// apply command
		switch op.Command {
		case CCHANGE:
			kv.applyChange(op, &res)
		case CDELETE:
			kv.applyDelete(op)
		case CGET:
			kv.applyGet(op, &res)
		case CPUT:
			kv.applyPut(op, &res)
		case CAPPEND:
			kv.applyAppend(op, &res)
		}

		// push to channel
		_, ok := kv.chans[com.CommandIndex]
		if !ok {
			kv.chans[com.CommandIndex] = make(chan Result, 1)
		}
		kv.chans[com.CommandIndex] <- res
		kv.mu.Unlock()
	}
}
