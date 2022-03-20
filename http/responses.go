package http

import (
	"github.com/iden3/reverse-hash-service/hashdb"
)

type nodeResponse struct {
	Node   hashdb.Node `json:"node"`
	Status string      `json:"status"`
}
