package sync

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ipfs/testground/sdk/runtime"

	"github.com/go-redis/redis"
)

const (
	EnvRedisHost  = "REDIS_HOST"
	EnvRedisPort  = "REDIS_PORT"
	RedisHostname = "testground-redis"
	HostHostname  = "host.docker.internal"
)

// redisClient returns a consul client from this processes environment
// variables, or panics if unable to create one.
//
// TODO: source redis URL from environment variables. The Redis host and port
// will be wired in by Nomad/Swarm.
func redisClient(runenv *runtime.RunEnv) (client *redis.Client, err error) {
	var (
		host = os.Getenv(EnvRedisHost)
		port = os.Getenv(EnvRedisPort)
	)

	// Try to resolve the "testground-redis" host from Docker's DNS first.
	// Fall back to attempting to use `host.docker.internal` which is
	// only available in macOS and Windows.
	for _, h := range []string{RedisHostname, HostHostname} {
		if addrs, err := net.LookupHost(h); err == nil && len(addrs) > 0 {
			host = h
			break
		}
	}

	if host == "" {
		// if none of these is available, try to use localhost in a desperate
		// attempt to make it work (useful for local:exec runners).
		host = "localhost"
	}

	if port == "" {
		port = "6379"
	}

	// TODO: will need to populate opts from an env variable.
	opts := &redis.Options{
		Addr:        fmt.Sprintf("%s:%s", host, port),
		MaxRetries:  3,
		ReadTimeout: 10 * time.Second,
	}

	client = redis.NewClient(opts)

	// PING redis to make sure we're alive.
	return client, client.Ping().Err()
}

// MustWatcherWriter proxies to WatcherWriter, panicking if an error occurs.
func MustWatcherWriter(runenv *runtime.RunEnv) (*Watcher, *Writer) {
	watcher, writer, err := WatcherWriter(runenv)
	if err != nil {
		panic(err)
	}
	return watcher, writer
}

// WatcherWriter creates a Watcher and a Writer object associated with this test
// run's sync tree.
func WatcherWriter(runenv *runtime.RunEnv) (*Watcher, *Writer, error) {
	watcher, err := NewWatcher(runenv)
	if err != nil {
		return nil, nil, err
	}

	writer, err := NewWriter(runenv)
	if err != nil {
		return nil, nil, err
	}

	return watcher, writer, nil
}

func basePrefix(runenv *runtime.RunEnv) string {
	p := fmt.Sprintf("run:%s:plan:%s:case:%s", runenv.TestRun, runenv.TestPlan, runenv.TestCase)
	return p
}

// seqFromKey extracts the seq counter from the key. If the last token is not
// an seq int value, it panics.
func seqFromKey(key string) int {
	splt := strings.Split(key, ":")
	seq, err := strconv.Atoi(splt[len(splt)-1])
	if err != nil {
		panic(err)
	}
	return seq
}

// decodePayload extracts a value of the specified type from incoming json.
func decodePayload(val interface{}, typ reflect.Type) (reflect.Value, error) {
	// Deserialize the value.
	payload := reflect.New(typ)
	raw, ok := val.(string)
	if !ok {
		panic("payload not a string")
	}
	if err := json.Unmarshal([]byte(raw), payload.Interface()); err != nil {
		return reflect.Value{}, fmt.Errorf("failed to decode as type %s: %s", typ, string(raw))
	}
	return payload, nil
}
