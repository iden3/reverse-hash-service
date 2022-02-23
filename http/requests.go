package http

import (
	"encoding/hex"
	"encoding/json"

	"github.com/iden3/go-merkletree-sql"
	"github.com/pkg/errors"
)

type node struct {
	hash, left, right merkletree.Hash
}

type nodeSubmitRequest []node

const (
	keyHash   = "hash"
	keyLeft   = "left"
	keyRight  = "right"
	keyStatus = "status"
	keyNode   = "node"
)

func (n *node) UnmarshalJSON(bytes []byte) error {
	var m map[string]interface{}
	err := json.Unmarshal(bytes, &m)
	if err != nil {
		return errors.WithStack(err)
	}

	n.hash = merkletree.HashZero
	n.left = merkletree.HashZero
	n.right = merkletree.HashZero

	for k, v := range m {
		switch k {
		case keyHash:
			err = unpackHash(&n.hash, v)
		case keyLeft:
			err = unpackHash(&n.left, v)
		case keyRight:
			err = unpackHash(&n.right, v)
		default:
			err = errors.Errorf("unknown key %v", k)
		}
		if err != nil {
			return err
		}
	}

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
