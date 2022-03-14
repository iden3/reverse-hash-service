CREATE TABLE mt_node (
    id BIGSERIAL PRIMARY KEY,
    hash BYTEA NOT NULL UNIQUE CHECK (length(hash) = 32),
    lchild BYTEA CHECK (length(lchild) = 32),
    rchild BYTEA CHECK (length(rchild) = 32)
);
