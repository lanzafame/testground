package main

import (
	"context"
	"fmt"

	// test "github.com/ipfs/testground/plans/smlbench2/test"
	// utils "github.com/ipfs/testground/plans/smlbench2/utils"
	iptb "github.com/ipfs/testground/sdk/iptb"
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Inventory of Tests
/*
var testCasesSet = [][]utils.SmallBenchmarksTestCase{
	{
		&test.SimpleAddGetTC{10 * 1024 * 1024}, // 10mb
	},
}
*/

// TODO:
//  Testcase abstraction.
//  Entrypoint demuxing (TEST_CASE_SEQ).
//  Pipe stdout to intercept messages.
//  Temporary directory from environment variable.
//  Error handling -- right now everything panics on failure.
func main() {
	// _ = os.Setenv("TEST_PLAN", "smlbenchmarks")
	// _ = os.Setenv("TEST_BRANCH", "master")
	// _ = os.Setenv("TEST_TAG", "")
	// _ = os.Setenv("TEST_RUN", uuid.New().String())

	runenv := runtime.CurrentRunEnv()
	if runenv.TestCaseSeq < 0 {
		panic("test case sequence number not set")
	}

	// testCases := testCasesSet[runenv.TestCaseSeq]

	watcher, writer := sync.MustWatcherWriter(runenv)
        defer watcher.Close()
	fmt.Printf("Jim1 writer %v\n", writer)

	spec := iptb.NewTestEnsembleSpec()
	spec.AddNodesDefaultConfig(iptb.NodeOpts{Initialize: true, Start: true}, "local")

	ctx := context.Background()
	ensemble := iptb.NewTestEnsemble(ctx, spec)
	ensemble.Initialize()

	localNode := ensemble.GetNode("local")

	fmt.Println("Jim localNode", localNode)
	fmt.Println("Jim", *localNode)
	fmt.Printf("Jim2 %T\n", *localNode)
	// fmt.Printf("Jim3 %v\n", *localNode.PeerID)
	peerID, err := localNode.PeerID()
	if err != nil {
		panic(err)
	}
	fmt.Println("Jim peerID", peerID)

	ID, err := peer.IDB58Decode(peerID)
	if err != nil {
		panic(err)
	}
	addrInfo := &peer.AddrInfo{
		ID: ID,
	}
	fmt.Println("Jim addrInfo", addrInfo)

	seq, err := writer.Write(sync.PeerSubtree, addrInfo)
	if err != nil {
                panic(err)
        }
	fmt.Println("Jim seq", seq)
	/*
	if _, err = writer.Write(sync.PeerSubtree, host.InfoFromHost(h)); err != nil {
                panic(err)
        }
	*/
        defer writer.Close()

	client := localNode.Client()
	fmt.Printf("Jim client %v\n", client)
	/*
	*/

	fmt.Println("Jim Run test here")

	/*
	for _, tc := range testCases {

		tc.Execute(runenv, ensemble)

		ensemble.Destroy()
	}
	*/
	ensemble.Destroy()
}
