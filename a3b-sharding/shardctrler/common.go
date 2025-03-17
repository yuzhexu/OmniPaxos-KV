package shardctrler

//
// Shard controler: assigns shards to replication groups.
//
// RPC interface:
// Join(servers) -- add a set of groups (gid -> server-list mapping).
// Leave(gids) -- delete a set of groups.
// Move(shard, gid) -- hand off one shard from current owner to gid.
// Query(num) -> fetch Config # num, or latest config if num==-1.
//
// A Config (configuration) describes a set of replica groups, and the
// replica group responsible for each shard. Configs are numbered. Config
// #0 is the initial configuration, with no groups and all shards
// assigned to group 0 (the invalid group).
//
// You will need to add fields to the RPC argument structs.
//

// The number of shards.
const NShards = 10

// A configuration -- an assignment of shards to groups.
// Please don't change this.
type Config struct {
	Num    int
	Shards [NShards]int
	Groups map[int][]string
}

const (
	Leave = "LeaveCommand"
	Move  = "MoveCommand"
	Join  = "JoinCommand"
	Query = "QueryCommand"
)

type QueryArgs struct {
	RequestId int64
	ConfNum   int
	ClientId  int64
}

type QueryReply struct {
	Err    string
	Config Config
}

type Result struct {
	RequestId int64
	Err       string
	Config    Config
	ClientId  int64
}

type JoinArgs struct {
	RequestId int64
	Servers   map[int][]string
	ClientId  int64
}

type JoinReply struct {
	Err string
}

type LeaveArgs struct {
	RequestId int64
	GIDs      []int
	ClientId  int64
}

type LeaveReply struct {
	Err string
}

type MoveArgs struct {
	RequestId int64
	Shard     int
	GID       int
	ClientId  int64
}

type MoveReply struct {
	Err string
}
