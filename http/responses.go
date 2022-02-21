package http

import (
	"encoding/hex"

	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
)

type nodeResponse struct {
	hashdb.Node
}

func (n nodeResponse) MarshalJSON() ([]byte, error) {
	switch nt := n.Node.(type) {
	case hashdb.Leaf:
		return []byte(
			`{"` + keyHash + `":"` + hex.EncodeToString(nt[:]) + `"}`), nil
	case hashdb.MiddleNode:
		return []byte(`{"` +
			keyHash + `":"` + hex.EncodeToString(nt.Hash[:]) + `","` +
			keyRight + `":"` + hex.EncodeToString(nt.Right[:]) + `","` +
			keyLeft + `":"` + hex.EncodeToString(nt.Left[:]) + `"}`), nil
	}
	return nil, errors.New("unexpected node, can't marshal to json")
}
