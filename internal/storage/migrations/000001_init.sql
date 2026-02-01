-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    login varchar(255) NOT NULL UNIQUE,
    password TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    number varchar(200) NOT NULL,
    status varchar(200) NOT NULL,
    accrual DOUBLE PRECISION DEFAULT 0,
    uploaded_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS withdrawals (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    order_number varchar(200) NOT NULL,
    sum DOUBLE PRECISION NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd