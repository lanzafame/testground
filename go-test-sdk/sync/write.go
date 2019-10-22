package sync

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ipfs/testground/sdk/runtime"

	"github.com/go-redis/redis"
)

var (
	// TTL is the expiry of the records this writer inserts.
	TTL = 10 * time.Second

	// KeepAlivePeriod is half the TTL. The Writer extends the TTL of the
	// records it owns with this frequency.
	KeepAlivePeriod = TTL / 2
)

// Writer offers an API to write objects to the sync tree for a running test.
//
// The sync service is designed to run in a distributed test system, where
// things will fail and end ungracefully. To avoid an ever-growing population of
// leftover records in Redis, all keys we write have a TTL, as per the sync.TTL
// var. We keep custody of these keys by periodically extending the TTL while
// the test instance is running. See godoc on keepAlive* methods and struct
// fields for more information.
type Writer struct {
	lk     sync.RWMutex
	client *redis.Client
	doneCh chan struct{}

	// root is the namespace under which this test run writes. It is derived
	// from the RunEnv.
	root string

	// ownSet tracks the keys written and owned by this Writer, grouped by
	// GroupKey. We drop these keys when stopping gracefully, i.e. when Close()
	// is called.
	ownSet map[string][]string

	// keepAliveSet are the keys we are responsible for keeping alive. This is a
	// superset of ownset + special keys we are responsible for keeping alive.
	keepAliveSet map[string]struct{}
}

// NewWriter creates a new Writer for a specific test run, as defined by the
// RunEnv.
func NewWriter(runenv *runtime.RunEnv) (w *Writer, err error) {
	client, err := redisClient(runenv)
	if err != nil {
		return nil, err
	}

	w = &Writer{
		client:       client,
		root:         basePrefix(runenv),
		doneCh:       make(chan struct{}),
		ownSet:       make(map[string][]string),
		keepAliveSet: make(map[string]struct{}),
	}

	// Start a background worker that keeps alive the keeys
	go w.keepAliveWorker()
	return w, nil
}

// keepAliveWorker runs a loop that extends the TTL in the keepAliveSet every
// `KeepAlivePeriod`. It should be launched as a goroutine.
func (w *Writer) keepAliveWorker() {
	for {
		select {
		case <-time.After(KeepAlivePeriod):
			w.keepAlive()
		case <-w.doneCh:
			return
		}
	}
}

// keepAlive extends the TTL of all keys in the keepAliveSet.
func (w *Writer) keepAlive() {
	w.lk.RLock()
	defer w.lk.RUnlock()

	// TODO: do this in a transaction. We risk the loop overlapping with the
	// refresh period, and all kinds of races. We need to be adaptive here.
	for k, _ := range w.keepAliveSet {
		if err := w.client.Expire(k, TTL).Err(); err != nil {
			panic(err)
		}
	}
}

// Write writes a payload in the sync tree for the test.
//
// It _panics_ if the payload's type does not match the expected type for the
// subtree.
//
// If the actual write on the sync service fails, this method returns an error.
//
// Else, if all succeeds, it returns the ordinal sequence number of this entry
// within the subtree.
func (w *Writer) Write(subtree *Subtree, payload interface{}) (seq int64, err error) {
	if err = subtree.AssertType(reflect.ValueOf(payload).Type()); err != nil {
		return -1, err
	}

	// Serialize the payload.
	bytes, err := json.Marshal(payload)
	if err != nil {
		return -1, err
	}

	// Calculate the index key. This key itself holds a Redis SET enumerating
	// all children. The seq ordinal and the actual leaf nodes are nested using
	// this key as a prefix.
	idx := w.root + ":" + subtree.GroupKey

	// Claim an seq by incrementing the :seq child key.
	seq, err = w.client.Incr(idx + ":seq").Result()
	if err != nil {
		return -1, err
	}

	// If we are within the first 5 nodes writing to this subtree, we're
	// responsible for keeping the index key alive. Having _all_ nodes
	// refreshing the index keys would be wasteful, so selecting a few
	// supervisors deterministically is appropriate.
	//
	// TODO(raulk) this is not entirely sound. In some test choreographies, the
	// first 5 nodes may legitimately finish first (or they may be biased in
	// some other manner), in which case these keys would cease being refreshed,
	// and they'd expire early. The situation is both improbable, and unlikely
	// to become a problem. Let's cross that bridge when we get to it.
	if seq <= 5 {
		w.lk.Lock()
		w.keepAliveSet[idx] = struct{}{}
		w.lk.Unlock()
	}

	// Payload key segments:
	// run:<runid>:plan:<plan_name>:case:<case_name>:<group_key>:<payload_key>:<seq>
	// e.g.
	// run:123:plan:dht:case:lookup_peers:nodes:QmPeer:417
	payloadKey := strings.Join([]string{idx, subtree.KeyFunc(payload), strconv.Itoa(int(seq))}, ":")

	// Perform a Redis transaction setting the payload key, adding it to the
	// index set, and extending the expiry of the latter.
	err = w.client.Watch(func(tx *redis.Tx) error {
		_, err := tx.Pipelined(func(pipe redis.Pipeliner) error {
			pipe.Set(payloadKey, bytes, TTL)
			pipe.SAdd(idx, payloadKey)
			pipe.Expire(idx, TTL)
			return nil
		})
		return err
	})

	if err != nil {
		return -1, err
	}

	// Update the ownset and refreshset.
	w.lk.Lock()
	os := w.ownSet[subtree.GroupKey]
	w.ownSet[subtree.GroupKey] = append(os, payloadKey)
	w.keepAliveSet[payloadKey] = struct{}{}
	w.lk.Unlock()

	return seq, err
}

// SignalEntry signals entry into the specified state, and returns how many
// instances are currently in this state, including the caller.
func (w *Writer) SignalEntry(s State) (current int64, err error) {
	// Increment a counter on the state key.
	key := strings.Join([]string{w.root, "states", string(s)}, ":")
	seq, err := w.client.Incr(key).Result()

	if err != nil {
		return -1, err
	}

	// If we're within the first 5 instances to write to this state key, we're a
	// supervisor and responsible for keeping it alive. See comment on the
	// analogous logic in Write() for more context.
	if seq <= 5 {
		w.lk.Lock()
		w.keepAliveSet[key] = struct{}{}
		w.lk.Unlock()
	}
	return seq, err
}

// Close closes this Writer, and drops all owned keys immediately, erroring if
// those deletions fail.
func (w *Writer) Close() error {
	close(w.doneCh)

	w.lk.Lock()
	defer w.lk.Unlock()

	// Drop all keys owned by this writer.
	for g, os := range w.ownSet {
		if err := w.client.SRem(g, os).Err(); err != nil {
			return err
		}
		if err := w.client.Del(os...).Err(); err != nil {
			return err
		}
	}

	w.ownSet = nil
	w.keepAliveSet = nil

	return nil
}
