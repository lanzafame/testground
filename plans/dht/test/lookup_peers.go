package test

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dhtopts "github.com/libp2p/go-libp2p-kad-dht/opts"

	"github.com/ipfs/go-datastore"
)

func LookupPeers(runenv *runtime.RunEnv) {
	timeout := func() time.Duration {
		if t, ok := runenv.IntParam("timeout_secs"); !ok {
			return 30 * time.Second
		} else {
			return time.Duration(t) * time.Second
		}
	}()

	// TODO Make parameter getting nicer by providing typed accessors in the
	// RunEnv (à la urfave/cli), that accept a second argument (default value).
	bucketSize := func() int {
		if t, ok := runenv.IntParam("bucket_size"); !ok {
			return 20
		} else {
			return t
		}
	}()

	// moot assignment to pacify the compiler. We need to merge the configurable
	// bucket size param upstream.
	_ = bucketSize

	h, err := libp2p.New(context.Background())
	if err != nil {
		panic(err)
	}

	dht, err := dht.New(context.Background(), h, dhtopts.Datastore(datastore.NewMapDatastore()))
	if err != nil {
		panic(err)
	}

	watcher, writer := sync.MustWatcherWriter(runenv)
	defer watcher.Close()

	if _, err = writer.Write(sync.PeerSubtree, host.InfoFromHost(h)); err != nil {
		panic(err)
	}
	defer writer.Close()

	peerCh := make(chan *peer.AddrInfo, 16)
	cancel, err := watcher.Subscribe(sync.PeerSubtree, sync.TypedChan(peerCh))
	defer cancel()

	var events int
	for i := 0; i < runenv.TestInstanceCount; i++ {
		select {
		case ai := <-peerCh:
			events++
			if ai.ID == h.ID() {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := h.Connect(ctx, *ai)
			if err != nil {
				panic(err)
			}
			cancel()

		case <-time.After(timeout):
			// TODO need a way to fail a distributed test immediately. No point
			// making it run elsewhere beyond this point.
			panic(fmt.Sprintf("no new peers in %d seconds", timeout))
		}
	}

	for i, id := range h.Peerstore().PeersWithAddrs() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		t := time.Now()
		if _, err := dht.FindPeer(ctx, id); err != nil {
			cancel()
			runenv.Abort(err)
			return
		}

		runenv.EmitMetric(&runtime.MetricDefinition{
			Name:           fmt.Sprintf("time-to-find-%d", i),
			Unit:           "ns",
			ImprovementDir: -1,
		}, float64(time.Now().Sub(t).Nanoseconds()))

		cancel()
	}

	ctx, ctxCancel := context.WithTimeout(context.Background(), timeout)
	defer ctxCancel()

	end := sync.State("end")

	// Set a state barrier.
	doneCh := watcher.Barrier(ctx, end, int64(runenv.TestInstanceCount))

	// Signal we're done on the end state.
	_, err = writer.SignalEntry(end)
	if err != nil {
		panic(err)
	}

	// Wait until all others have signalled.
	if err := <-doneCh; err != nil {
		panic(err)
	}

	runenv.OK()
}
