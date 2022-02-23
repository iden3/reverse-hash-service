package http

import (
	"encoding/hex"
	"encoding/json"

	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
)

type nodeResponse struct {
	node   hashdb.Node
	status string
}

func (n nodeResponse) MarshalJSON() ([]byte, error) {
	node := map[string]interface{}{}
	switch nt := n.node.(type) {
	case hashdb.Leaf:
		node[keyHash] = hexHash(nt)
	case hashdb.MiddleNode:
		node[keyHash] = hexHash(nt.Hash)
		node[keyLeft] = hexHash(nt.Left)
		node[keyRight] = hexHash(nt.Right)
	}

	resp := map[string]interface{}{keyStatus: n.status, keyNode: node}
	respBytes, err := json.Marshal(resp)
	return respBytes, errors.WithStack(err)
}

type hexHash merkletree.Hash

func (h hexHash) MarshalJSON() ([]byte, error) {
	return []byte(`"` + hex.EncodeToString(h[:]) + `"`), nil
}
