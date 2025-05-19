package http

import (
	"encoding/hex"
	"encoding/json"

	"github.com/iden3/go-merkletree-sql"
	"github.com/iden3/reverse-hash-service/hashdb"
	"github.com/pkg/errors"
)

type nodeSubmitRequest []hashdb.Node

const (
	keyStatus = "status"
	keyError  = "error"
)

func (n *nodeSubmitRequest) UnmarshalJSON(bytes []byte) error {
	var objList []json.RawMessage
	err := json.Unmarshal(bytes, &objList)
	if err != nil {
		return errors.WithStack(err)
	}

	*n = make(nodeSubmitRequest, len(objList))
	nodes := make([]hashdb.Node, len(objList))
	for i := range objList {
		err := json.Unmarshal(objList[i], &nodes[i])
		if err != nil {
			return errors.Wrapf(err, "error parsing node #%v", i+1)
		}
	}

	*n = nodes
	return nil
}

func unpackHash(h *merkletree.Hash, i interface{}) error {
	s, ok := i.(string)
	if !ok {
		return errors.Errorf("value expected to be string, got %T", i)
	}
	if len(s) != len(h[:])*2 {
		return errors.Errorf("length of hash should be %v", len(h[:])*2)
	}
	_, err := hex.Decode(h[:], []byte(s))
	return errors.WithStack(err)
}
