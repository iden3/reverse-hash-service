CREATE TABLE mt_node (
    id BIGSERIAL PRIMARY KEY,
    hash BYTEA NOT NULL UNIQUE,
    lchild BYTEA,
    rchild BYTEA
);