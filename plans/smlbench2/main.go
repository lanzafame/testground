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
)

const (
	sizeBytes = 10 * 1024 * 1024 // 10mb
)

func main() {
	runenv := runtime.CurrentRunEnv()
	if runenv.TestCaseSeq < 0 {
		panic("test case sequence number not set")
	}

	/*
	timeout := func() time.Duration {
                if t, ok := runenv.IntParam("timeout_secs"); !ok {
                        return 30 * time.Second
                } else {
                        return time.Duration(t) * time.Second
                }
        }()
	*/

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

	ID, err := peer.IDB58Decode(peerID)
	if err != nil {
		runenv.Abort(err)
	}
	addrInfo := &peer.AddrInfo{
		ID: ID,
	}

	seq, err := writer.Write(sync.PeerSubtree, addrInfo)
	if err != nil {
                runenv.Abort(err)
        }
        defer writer.Close()

	client := localNode.Client()

	added := sync.State("added")

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

		/*
		ctx, ctxCancel := context.WithTimeout(context.Background(), timeout)
		defer ctxCancel()
		*/

		fmt.Println("Sleeping...")
		time.Sleep(5 * time.Second)

		// Signal we're done on the added state.
		_, err = writer.SignalEntry(added)
		if err != nil {
			runenv.Abort(err)
		}
		fmt.Println("Done add")
	case seq == 2: // getter
		getter := client
		fmt.Println("getter")

		// Set a state barrier.
		addedCh := watcher.Barrier(ctx, added, 1)

		// Wait until added state is signalled.
		if err := <-addedCh; err != nil {
			panic(err)
		}
		fmt.Println("Done add")
		fmt.Println("Jim getter", getter)
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
