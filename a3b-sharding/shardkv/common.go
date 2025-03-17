package shardkv

//
// Sharded key/value server.
// Lots of replica groups, each running OmniPaxos.
// Shardctrler decides which group serves each shard.
// Shardctrler may change shard assignment from time to time.
//
// You will have to modify these definitions.
//

type Err string

const (
	OK            = "OK"
	ErrNoKey      = "ErrNoKey"
	ErrWrongGroup = "ErrWrongGroup"
	ErrNotLeader  = "ErrNotLeader"
	ErrTryAgain   = "ErrTryAgain"
	ErrTimeout    = "ErrTimeout"
)

// Put or Append
type PutAppendArgs struct {
	// You'll have to add definitions here.
	Key   string
	Value string
	Op    string
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	ClientId  int64
	RequestId int64
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	Key string
	// You'll have to add definitions here.
	ClientId  int64
	RequestId int64
}

type GetReply struct {
	Value string
	Err   Err
}

type DeleteArgs struct {
	Num     int
	ShardId int
}

type DeleteReply struct {
	Err Err
}

type TransferArgs struct {
	Num      int
	ShardIds []int
}

type TransferReply struct {
	Store [NShards]map[string]string
	Prev  map[int64]int64
	Err   Err
}
