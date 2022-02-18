package hashdb

import (
	"context"
	stderr "errors"
	"fmt"

	"github.com/iden3/go-merkletree-sql"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/pkg/errors"
)

var ErrDoesNotExists = stderr.New("node does not exists")

type Leaf merkletree.Hash

func (l Leaf) Hash() merkletree.Hash {
	return merkletree.Hash(l)
}

func (l Leaf) IsLeaf() bool {
	return true
}

type MiddleNode struct {
	hash  merkletree.Hash
	left  merkletree.Hash
	right merkletree.Hash
}

func (m MiddleNode) Hash() merkletree.Hash {
	return m.hash
}

func (m MiddleNode) IsLeaf() bool {
	return false
}

func (m MiddleNode) calcHash() (merkletree.Hash, error) {
	h, err := merkletree.HashElems(m.left.BigInt(), m.right.BigInt())
	if err != nil {
		return merkletree.Hash{}, errors.WithStack(err)
	}
	return *h, nil
}

type Node interface {
	Hash() merkletree.Hash
	IsLeaf() bool
}

type Storage interface {
	SaveMiddleNode(ctx context.Context, node MiddleNode) (bool, error)
	SaveLeaf(ctx context.Context, node Leaf) (bool, error)
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
}

type pgStorage struct {
	db dbI
}

// SaveMiddleNode inserts middle node into database. If node already exists
// return false. If inserted, return true.
func (p *pgStorage) SaveMiddleNode(ctx context.Context,
	node MiddleNode) (bool, error) {

	calcedHash, err := node.calcHash()
	if err != nil {
		return false, errors.WithStack(err)
	}

	if calcedHash != node.hash {
		return false, errors.New("node hash is not correct")
	}

	var pgHash, pgLeft, pgRight pgtype.Bytea
	if err := pgHash.Set(node.hash[:]); err != nil {
		return false, errors.WithStack(err)
	}
	if err := pgLeft.Set(node.left[:]); err != nil {
		return false, errors.WithStack(err)
	}
	if err := pgRight.Set(node.right[:]); err != nil {
		return false, errors.WithStack(err)
	}

	var id pgtype.Int8
	query := fmt.Sprintf(
		`
INSERT INTO %[1]v (hash, lchild, rchild) VALUES ($1, $2, $3)
ON CONFLICT (hash) DO NOTHING
RETURNING id`,
		quote(tableMtNode))
	err = p.db.QueryRow(ctx, query, pgHash, pgLeft, pgRight).Scan(&id)
	switch err {
	case pgx.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, errors.WithStack(err)
	}
}

func (p *pgStorage) SaveLeaf(ctx context.Context, node Leaf) (bool, error) {
	var pgHash pgtype.Bytea
	if err := pgHash.Set(node[:]); err != nil {
		return false, errors.WithStack(err)
	}
	var id pgtype.Int8
	query := fmt.Sprintf(
		`
INSERT INTO %[1]v (hash) VALUES ($1)
ON CONFLICT (hash) DO NOTHING
RETURNING id`,
		quote(tableMtNode))
	err := p.db.QueryRow(ctx, query, pgHash).Scan(&id)
	switch err {
	case pgx.ErrNoRows:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, errors.WithStack(err)
	}
}

func (p *pgStorage) ByHash(ctx context.Context,
	hash merkletree.Hash) (Node, error) {

	var pgHash, left, right pgtype.Bytea
	err := pgHash.Set(hash[:])
	if err != nil {
		return nil, errors.WithStack(err)
	}
	query := fmt.Sprintf(
		`SELECT lchild, rchild FROM %[1]v WHERE hash = $1`,
		quote(tableMtNode))
	switch err = p.db.QueryRow(ctx, query, pgHash).Scan(&left, &right); err {
	case pgx.ErrNoRows:
		return nil, errors.WithStack(ErrDoesNotExists)
	case nil:
	default:
		return nil, errors.WithStack(err)
	}

	if left.Status == pgtype.Present && right.Status == pgtype.Present {
		middleNode := MiddleNode{hash: hash}
		var childHash []byte

		err = left.AssignTo(&childHash)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		copy(middleNode.left[:], childHash)

		err = right.AssignTo(&childHash)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		copy(middleNode.right[:], childHash)
		return middleNode, nil
	} else if left.Status == pgtype.Null && right.Status == pgtype.Null {
		return Leaf(hash), nil
	} else {
		return nil, errors.New("[assertion] unexpected node type")
	}
}

func quote(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}

func New(db *pgxpool.Pool) Storage {
	return &pgStorage{db}
}
