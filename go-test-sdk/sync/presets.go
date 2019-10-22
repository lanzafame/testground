package sync

import (
	"reflect"

	"github.com/libp2p/go-libp2p-core/peer"
)

type Action int

const (
	ActionAdded Action = iota
	ActionDeleted
)

type PeerPayload struct {
	Action
	*peer.AddrInfo
}

// PeerSubtree represents a subtree under the test run's sync tree where peers
// participating in this distributed test advertise themselves.
var PeerSubtree = &Subtree{
	GroupKey:    "nodes",
	PayloadType: reflect.TypeOf(&peer.AddrInfo{}),
	KeyFunc: func(val interface{}) string {
		return val.(*peer.AddrInfo).ID.Pretty()
	},
}
