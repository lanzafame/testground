package main

import (
	"context"
	"fmt"

	test "github.com/ipfs/testground/plans/smlbench2/test"
	utils "github.com/ipfs/testground/plans/smlbench2/utils"
	iptb "github.com/ipfs/testground/sdk/iptb"
	"github.com/ipfs/testground/sdk/runtime"
	"github.com/ipfs/testground/sdk/sync"
)

// Inventory of Tests
var testCasesSet = [][]utils.SmallBenchmarksTestCase{
	{
		&test.SimpleAddGetTC{10 * 1024 * 1024}, // 10mb
	},
}

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

	testCases := testCasesSet[runenv.TestCaseSeq]

	watcher, writer := sync.MustWatcherWriter(runenv)
        defer watcher.Close()
	fmt.Printf("Jim1 writer %v\n", writer)

	for _, tc := range testCases {
		ctx := context.Background()

		spec := iptb.NewTestEnsembleSpec()
		tc.Configure(runenv, spec)

		ensemble := iptb.NewTestEnsemble(ctx, spec)
		ensemble.Initialize()

		tc.Execute(runenv, ensemble)

		ensemble.Destroy()
	}
}
