CREATE SCHEMA IF NOT EXISTS optimaldn;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE ROLE "user" WITH LOGIN PASSWORD 'password';
GRANT ALL PRIVILEGES ON DATABASE optimaldn TO "user";


CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL
);

INSERT INTO users (id, username, password_hash)
VALUES (uuid_generate_v4(), 'admin', '$2a$10$ejf4Oyyw7/CX4IaIhh7AxO8453gDvjeYwkKKmNnXOsGqxgmCB9Ed2')
ON CONFLICT (username) DO NOTHING;

CREATE TABLE IF NOT EXISTS user_saved_routes (
    route_id UUID PRIMARY KEY,
    user_id TEXT NOT NULL,
    start_point TEXT NOT NULL,
    end_point TEXT NOT NULL,
    transport_mode TEXT,
    stops INTEGER,
    estimated_time INTEGER,
    line_names TEXT[],
    stops_names TEXT[]
);