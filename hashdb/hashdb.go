package hashdb

import (
	"bytes"
	"context"
	stderr "errors"
	"fmt"

	"github.com/iden3/go-merkletree-sql"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	"github.com/jackc/pgx/v4"
	"github.com/pkg/errors"
)

var ErrDoesNotExists = stderr.New("node does not exists")
var ErrIncorrectHash = stderr.New("node hash is not correct")
var ErrAlreadyExists = stderr.New("node already exists")
var ErrLeafUpgraded = stderr.New("leaf upgraded to middle node")
var ErrCollision = stderr.New("node hash collision")
var ErrMiddleNodeExists = stderr.New("middle node exists with same hash")

type Leaf merkletree.Hash

func (l Leaf) Hash() merkletree.Hash {
	return merkletree.Hash(l)
}

func (l Leaf) IsLeaf() bool {
	return true
}

type MiddleNode struct {
	Hash  merkletree.Hash
	Left  merkletree.Hash
	Right merkletree.Hash
}

func (m MiddleNode) IsLeaf() bool {
	return false
}

func (m MiddleNode) calcHash() (merkletree.Hash, error) {
	h, err := merkletree.HashElems(m.Left.BigInt(), m.Right.BigInt())
	if err != nil {
		return merkletree.Hash{}, errors.WithStack(err)
	}
	return *h, nil
}

type Node interface {
	IsLeaf() bool
}

type Storage interface {
	SaveMiddleNode(ctx context.Context, node MiddleNode) error
	SaveLeaf(ctx context.Context, node Leaf) error
	ByHash(ctx context.Context, hash merkletree.Hash) (Node, error)
}

type pgChildren struct{ left, right pgtype.Bytea }

const (
	tableMtNode = "mt_node"
)

// discriminators to distinguish insert from selected rows from DB
const (
	opInsert = "i"
	opSelect = "s"
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

// SaveMiddleNode inserts middle node into database.
// Possible errors:
//   * ErrIncorrectHash — node hash does not match to hash of children
//   * ErrAlreadyExists — this node already exists in database
//   * ErrLeafUpgraded  — there was a leaf in a database, and it was upgraded to
//                        middle node
//   * ErrCollision     — another node with this hash but different children
//                        found in database
func (p *pgStorage) SaveMiddleNode(ctx context.Context,
	node MiddleNode) error {

	calcedHash, err := node.calcHash()
	if err != nil {
		return errors.WithStack(err)
	}

	if calcedHash != node.Hash {
		return errors.WithStack(ErrIncorrectHash)
	}

	var pgHash, pgLeft, pgRight pgtype.Bytea
	if err := pgHash.Set(node.Hash[:]); err != nil {
		return errors.WithStack(err)
	}
	if err := pgLeft.Set(node.Left[:]); err != nil {
		return errors.WithStack(err)
	}
	if err := pgRight.Set(node.Right[:]); err != nil {
		return errors.WithStack(err)
	}

	query := fmt.Sprintf(`
WITH
  ins AS (
    INSERT INTO %[1]v (hash, lchild, rchild) VALUES ($1, $2, $3)
    ON CONFLICT (hash) DO UPDATE
    SET lchild = $2, rchild = $3
    WHERE %[1]v.lchild IS NULL AND %[1]v.rchild IS NULL
    RETURNING $4, lchild, rchild
)
SELECT * FROM ins
UNION ALL
SELECT $5, lchild, rchild FROM %[1]v WHERE hash = $1`,
		quote(tableMtNode))
	rows, err := p.db.Query(ctx, query, pgHash, pgLeft, pgRight, opInsert,
		opSelect)
	if err != nil {
		return errors.WithStack(err)
	}
	defer rows.Close()

	results := map[string]pgChildren{}

	for rows.Next() {
		var c pgChildren
		var op string
		if err := rows.Scan(&op, &c.left, &c.right); err != nil {
			return errors.WithStack(err)
		}
		results[op] = c
	}

	if err = rows.Err(); err != nil {
		return errors.WithStack(err)
	}

	iRow, iRowExists := results[opInsert]
	sRow, sRowExists := results[opSelect]

	switch {
	// 1: added a new row into database
	case !sRowExists && iRowExists &&
		iRow.left.Status == pgtype.Present &&
		bytes.Equal(iRow.left.Bytes, node.Left[:]) &&
		iRow.right.Status == pgtype.Present &&
		bytes.Equal(iRow.right.Bytes, node.Right[:]):
		return nil
	// 2: upgraded leaf to middle node
	case sRowExists && iRowExists &&
		iRow.left.Status == pgtype.Present &&
		bytes.Equal(iRow.left.Bytes, node.Left[:]) &&
		iRow.right.Status == pgtype.Present &&
		bytes.Equal(iRow.right.Bytes, node.Right[:]) &&
		sRow.left.Status == pgtype.Null &&
		sRow.right.Status == pgtype.Null:
		return errors.WithStack(ErrLeafUpgraded)
	// 3: node hash collision
	case sRowExists && !iRowExists &&
		sRow.left.Status == pgtype.Present &&
		sRow.right.Status == pgtype.Present &&
		(!bytes.Equal(sRow.left.Bytes, node.Left[:]) ||
			!bytes.Equal(sRow.right.Bytes, node.Right[:])):
		return errors.WithStack(ErrCollision)
	// 4: this node already exists
	case sRowExists && !iRowExists &&
		sRow.left.Status == pgtype.Present &&
		sRow.right.Status == pgtype.Present &&
		bytes.Equal(sRow.left.Bytes, node.Left[:]) &&
		bytes.Equal(sRow.right.Bytes, node.Right[:]):
		return errors.WithStack(ErrAlreadyExists)
	// unexpected case
	default:
		return errors.New("[assertion] unexpected middle node insertion case")
	}
}

// SaveLeaf inserts leaf node into database.
// Possible errors:
//   * ErrAlreadyExists    — this node already exists in database
//   * ErrMiddleNodeExists — middle node with the same hash exists in database
func (p *pgStorage) SaveLeaf(ctx context.Context, node Leaf) error {
	var pgHash pgtype.Bytea
	if err := pgHash.Set(node[:]); err != nil {
		return errors.WithStack(err)
	}

	query := fmt.Sprintf(`
WITH
  ins AS (
    INSERT INTO %[1]v (hash) VALUES ($1)
    ON CONFLICT (hash) DO NOTHING
    RETURNING $2, lchild, rchild
)
SELECT * FROM ins
UNION ALL
SELECT $3, lchild, rchild FROM %[1]v WHERE hash = $1`,
		quote(tableMtNode))
	rows, err := p.db.Query(ctx, query, pgHash, opInsert, opSelect)
	if err != nil {
		return errors.WithStack(err)
	}
	results := map[string]pgChildren{}

	for rows.Next() {
		var c pgChildren
		var op string
		if err := rows.Scan(&op, &c.left, &c.right); err != nil {
			return errors.WithStack(err)
		}
		results[op] = c
	}

	if err = rows.Err(); err != nil {
		return errors.WithStack(err)
	}

	iRow, iRowExists := results[opInsert]
	sRow, sRowExists := results[opSelect]

	switch {
	// 1: added a new row into database
	case !sRowExists && iRowExists &&
		iRow.left.Status == pgtype.Null &&
		iRow.right.Status == pgtype.Null:
		return nil
	// 2: this node already exists
	case sRowExists && !iRowExists &&
		sRow.left.Status == pgtype.Null &&
		sRow.right.Status == pgtype.Null:
		return errors.WithStack(ErrAlreadyExists)
	// 3: middle node exists with this hash
	case sRowExists && !iRowExists &&
		(sRow.left.Status == pgtype.Present ||
			sRow.right.Status == pgtype.Present):
		return errors.WithStack(ErrMiddleNodeExists)
	// unexpected case
	default:
		return errors.New("[assertion] unexpected leaf node insertion case")
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

	switch {
	case left.Status == pgtype.Present && right.Status == pgtype.Present:
		middleNode := MiddleNode{Hash: hash}
		var childHash []byte

		err = left.AssignTo(&childHash)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		copy(middleNode.Left[:], childHash)

		err = right.AssignTo(&childHash)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		copy(middleNode.Right[:], childHash)
		return middleNode, nil
	case left.Status == pgtype.Null && right.Status == pgtype.Null:
		return Leaf(hash), nil
	default:
		return nil, errors.New("[assertion] unexpected node type")
	}
}

func quote(identifier string) string {
	return pgx.Identifier{identifier}.Sanitize()
}

func New(db dbI) Storage {
	return &pgStorage{db}
}
