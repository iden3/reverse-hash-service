package hashdb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	stderr "errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/iden3/go-merkletree-sql"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

var ErrDoesNotExists = stderr.New("node does not exists")
var ErrIncorrectHash = stderr.New("node hash is not correct")

const (
	keyHash     = "hash"
	keyChildren = "children"
)

type Node struct {
	Hash     merkletree.Hash
	Children []merkletree.Hash
}

func (n Node) MarshalJSON() ([]byte, error) {
	var obj = make(map[string]interface{})
	obj[keyHash] = hex.EncodeToString(n.Hash[:])
	children := make([]string, len(n.Children))
	for i := range n.Children {
		children[i] = hex.EncodeToString(n.Children[i][:])
	}
	obj[keyChildren] = children
	bytes, err := json.Marshal(obj)
	return bytes, errors.WithStack(err)
}

func (n *Node) UnmarshalJSON(bytes []byte) error {
	var obj map[string]interface{}

	err := json.Unmarshal(bytes, &obj)
	if err != nil {
		return errors.WithStack(err)
	}

	hashI, ok := obj[keyHash]
	if !ok {
		return errors.Errorf("missing key: %v", keyHash)
	}
	hashS, ok := hashI.(string)
	if !ok {
		return errors.Errorf("'%v' value is not a string", keyHash)
	}
	hashB, err := hex.DecodeString(hashS)
	if err != nil {
		return errors.Errorf("error decoding %v: %v", keyHash, err)
	}
	if len(hashB) != len(n.Hash) {
		return errors.Errorf("'%v' value length is incorrect", keyHash)
	}
	copy(n.Hash[:], hashB)
	delete(obj, keyHash)

	childrenI, ok := obj[keyChildren]
	if !ok {
		return errors.Errorf("missing key: %v", keyChildren)
	}
	childrenL, ok := childrenI.([]interface{})
	if !ok {
		return errors.Errorf("'%v' value is not a list", keyChildren)
	}
	n.Children = make([]merkletree.Hash, len(childrenL))
	for i := range childrenL {
		childS, ok := childrenL[i].(string)
		if !ok {
			return errors.Errorf("child #%v is not a string", i)
		}
		childB, err := hex.DecodeString(childS)
		if err != nil {
			return errors.Errorf("error decoding child #%v: %v", i, err)
		}
		if len(n.Children[i]) != len(childB) {
			return errors.Errorf("incorrect length of child #%v", i)
		}
		copy(n.Children[i][:], childB)
	}
	delete(obj, keyChildren)

	for k := range obj {
		return errors.Errorf("unexpected key: %v", k)
	}

	wantHash, err := n.hashChildren()
	if err != nil {
		return err
	}
	if wantHash != n.Hash {
		return errors.WithStack(ErrIncorrectHash)
	}

	return nil
}

func (n Node) IsValid() (bool, error) {
	expectedHash, err := n.hashChildren()
	if err != nil {
		return false, err
	}
	return expectedHash == n.Hash, nil
}

func (n Node) hashChildren() (merkletree.Hash, error) {
	if len(n.Children) == 0 {
		return merkletree.HashZero, nil
	}
	var intValues = make([]*big.Int, len(n.Children))
	for i, c := range n.Children {
		intValues[i] = c.BigInt()
	}
	h, err := merkletree.HashElems(intValues...)
	return *h, errors.WithStack(err)
}

type Storage interface {
	SaveNodes(ctx context.Context, nodes []Node) error
	ByHash(ctx context.Context, hash merkletree.Hash) (Node, error)
}

const (
	tableMtNode = "mt_node"
)

type dbI interface {
	Query(ctx context.Context,
		sql string, args ...interface{}) (pgx.Rows, error)
	Exec(ctx context.Context,
		sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	BeginFunc(ctx context.Context, f func(pgx.Tx) error) error
}

type pgStorage struct {
	db dbI
}

const insertNodeChunkSize = 1000

// SaveNodes inserts leaf and middle nodes into database.
func (p *pgStorage) SaveNodes(ctx context.Context, nodes []Node) error {
	return p.db.BeginFunc(ctx, func(tx pgx.Tx) error {
		for i := 0; i < len(nodes); i += insertNodeChunkSize {
			maxIdx := i + insertNodeChunkSize
			if maxIdx > len(nodes) {
				maxIdx = len(nodes)
			}
			nodesChunk := nodes[i:maxIdx]
			sqlQuery, sqlParams, err := mkInsertNodesSQL(nodesChunk)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, sqlQuery, sqlParams...)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	})
}

func mkInsertNodesSQL(
	nodes []Node) (query string, params []interface{}, err error) {

	type sqlNode struct {
		hash     pgtype.Bytea
		children pgtype.ByteaArray
	}
	var sqlNodes = make([]sqlNode, len(nodes))
	var valid bool

	for i := range nodes {
		valid, err = nodes[i].IsValid()
		if err != nil {
			return
		}
		if !valid {
			err = errors.WithStack(ErrIncorrectHash)
			return
		}

		if nodes[i].Hash == merkletree.HashZero {
			err = errors.New("node hash zero hash")
			return
		}

		if err = sqlNodes[i].hash.Set(nodes[i].Hash[:]); err != nil {
			err = errors.WithStack(err)
			return
		}

		var children = make([]pgtype.Bytea, len(nodes[i].Children))
		for j := range nodes[i].Children {
			if err = children[j].Set(nodes[i].Children[j][:]); err != nil {
				err = errors.WithStack(err)
				return
			}
		}
		if err = sqlNodes[i].children.Set(children); err != nil {
			err = errors.WithStack(err)
			return
		}
	}

	var valuesStrs []string
	for i := range sqlNodes {
		valuesStrs = append(valuesStrs,
			fmt.Sprintf("($%v,$%v)", i*2+1, i*2+2))
		params = append(params, sqlNodes[i].hash, sqlNodes[i].children)
	}

	query = fmt.Sprintf(
		`
INSERT INTO %[1]v (hash, children)
VALUES %[2]v
ON CONFLICT DO NOTHING`,
		quote(tableMtNode), strings.Join(valuesStrs, ","))

	return query, params, nil
}

func (p *pgStorage) ByHash(ctx context.Context,
	hash merkletree.Hash) (Node, error) {

	var node = Node{Hash: hash}

	var pgChildren pgtype.ByteaArray
	var pgHash pgtype.Bytea
	err := pgHash.Set(hash[:])
	if err != nil {
		return node, errors.WithStack(err)
	}
	query := fmt.Sprintf(
		`SELECT children FROM %[1]v WHERE hash = $1`,
		quote(tableMtNode))
	switch err = p.db.QueryRow(ctx, query, pgHash).Scan(&pgChildren); err {
	case pgx.ErrNoRows:
		return node, errors.WithStack(ErrDoesNotExists)
	case nil:
	default:
		return node, errors.WithStack(err)
	}

	var children [][]byte
	if err := pgChildren.AssignTo(&children); err != nil {
		return node, errors.WithStack(err)
	}
	node.Children = make([]merkletree.Hash, len(children))
	for i := range children {
		if len(children[i]) != len(node.Children[i]) {
			return node, errors.New(
				"unexpected length of hash found in database")
		}
		copy(node.Children[i][:], children[i])
	}

	return node, nil
}

func quote(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}

func New(db dbI) Storage {
	return &pgStorage{db}
}
