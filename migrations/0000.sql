CREATE TABLE IF NOT EXISTS network (
    id TEXT PRIMARY KEY,
    endpoint TEXT,
    seed BLOB,
    pubkey BLOB[32],
	route TEXT
);

CREATE TABLE IF NOT EXISTS client (
    id TEXT PRIMARY KEY,
    network_id TEXT,
    ip TEXT,
    ifname TEXT,

    FOREIGN KEY(network_id) REFERENCES network(id) ON DELETE CASCADE
);
