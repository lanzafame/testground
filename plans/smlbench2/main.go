package main

import (
	"context"
	"fmt"
	"os"
	"time"

	utils "github.com/ipfs/testground/plans/smlbench2/utils"
	iptb "github.com/ipfs/testground/sdk/iptb"
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
	"github.com/libp2p/go-libp2p-core/peer"
	shell "github.com/ipfs/go-ipfs-api"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	sizeBytes = 10 * 1024 * 1024 // 10mb
)

func main() {
	runenv := runtime.CurrentRunEnv()
	if runenv.TestCaseSeq < 0 {
		panic("test case sequence number not set")
	}

	timeout := func() time.Duration {
                if t, ok := runenv.IntParam("timeout_secs"); !ok {
                        return 30 * time.Second
                } else {
                        return time.Duration(t) * time.Second
                }
        }()

	watcher, writer := sync.MustWatcherWriter(runenv)
        defer watcher.Close()

	spec := iptb.NewTestEnsembleSpec()
	spec.AddNodesDefaultConfig(iptb.NodeOpts{Initialize: true, Start: true}, "local")

	ctx := context.Background()
	ensemble := iptb.NewTestEnsemble(ctx, spec)
	ensemble.Initialize()

	localNode := ensemble.GetNode("local")

	peerID, err := localNode.PeerID()
	if err != nil {
		runenv.Abort(err)
	}
	fmt.Println("Jim peerID", peerID)

	swarmAddrs, err := localNode.SwarmAddrs()
	if err != nil {
		runenv.Abort(err)
	}
	fmt.Println("Jim swarmAddrs", swarmAddrs)

	ID, err := peer.IDB58Decode(peerID)
	if err != nil {
		runenv.Abort(err)
	}

	addrs := make([]ma.Multiaddr, len(swarmAddrs))
	for i, addr := range swarmAddrs {
		multiAddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			runenv.Abort(err)
		}
		addrs[i] = multiAddr
	}
	addrInfo := &peer.AddrInfo{ID, addrs}

	seq, err := writer.Write(sync.PeerSubtree, addrInfo)
	if err != nil {
                runenv.Abort(err)
        }
        defer writer.Close()

	client := localNode.Client()

	// States
	added := sync.State("added")
	received := sync.State("received")

	switch {
	case seq == 1: // adder
		adder := client
		fmt.Println("adder")

		// generate a random file of the designated size.
		file := utils.TempRandFile(runenv, ensemble.TempDir(), sizeBytes)
		defer os.Remove(file.Name())

		tstarted := time.Now()
		cid, err := adder.Add(file)
		if err != nil {
			runenv.Abort(err)
			return
		}
		fmt.Println("cid", cid)

		runenv.EmitMetric(utils.MetricTimeToAdd, float64(time.Now().Sub(tstarted)/time.Millisecond))

		// Signal we're done on the added state.
		_, err = writer.SignalEntry(added)
		if err != nil {
			runenv.Abort(err)
		}
		fmt.Println("State: added")

		// Set a state barrier.
		receivedCh := watcher.Barrier(ctx, received, 1)

		// Wait until recieved state is signalled.
		if err := <-receivedCh; err != nil {
			panic(err)
		}
		fmt.Println("State: received")
	case seq == 2: // getter
		getter := client
		fmt.Println("getter")

		// Connect to other peers
		peerCh := make(chan *peer.AddrInfo, 16)
		cancel, err := watcher.Subscribe(sync.PeerSubtree, sync.TypedChan(peerCh))
		if err != nil {
			runenv.Abort(err)
		}
		defer cancel()

		var events int
		fmt.Println("Jim testinstancecount", runenv.TestInstanceCount)
		for i := 0; i < runenv.TestInstanceCount; i++ {
			select {
			case ai := <-peerCh:
				events++
				fmt.Println("Jim connect1", events, ai)
				if ai.ID == ID {
					continue
				}
				fmt.Println("Jim connect2", ai)
				/*
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				err := h.Connect(ctx, *ai)
				if err != nil {
					panic(err)
				}
				*/
				cancel()

			case <-time.After(timeout):
				// TODO need a way to fail a distributed test immediately. No point
				// making it run elsewhere beyond this point.
				panic(fmt.Sprintf("no new peers in %d seconds", timeout))
			}
		}

		// Set a state barrier.
		addedCh := watcher.Barrier(ctx, added, 1)

		// Wait until added state is signalled.
		if err := <-addedCh; err != nil {
			panic(err)
		}
		fmt.Println("State: added")
		fmt.Println("Jim getter", getter)

		// Signal we're reached the received state.
		_, err = writer.SignalEntry(received)
		if err != nil {
			runenv.Abort(err)
		}
		fmt.Println("State: received")
	default:
		runenv.Abort(fmt.Errorf("Unexpected seq: %v", seq))
	}

	ensemble.Destroy()
}

func runAdder(runenv *runtime.RunEnv, ensemble *iptb.TestEnsemble, adder *shell.Shell, size int64) {
}

/*
func runGetter(getter *shell.Shell) {
	fmt.Println("getter")
}
*/
