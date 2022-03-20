CREATE TABLE mt_node (
    id BIGSERIAL PRIMARY KEY,
    hash BYTEA NOT NULL UNIQUE CHECK (length(hash) = 32),
    children BYTEA[]
);
