-- +goose Up
CREATE TABLE user_friends (
    user_id                INTEGER NOT NULL REFERENCES users(user_id),
    friend_player_id       INTEGER NOT NULL,
    became_friends_at      INTEGER NOT NULL DEFAULT 0,
    cheer_sent_today       INTEGER NOT NULL DEFAULT 0,
    cheer_received_pending INTEGER NOT NULL DEFAULT 0,
    stamina_received_today INTEGER NOT NULL DEFAULT 0,
    last_reset_day         INTEGER NOT NULL DEFAULT 0,
    latest_version         INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, friend_player_id)
);

CREATE TABLE user_friend_requests_incoming (
    user_id          INTEGER NOT NULL REFERENCES users(user_id),
    from_player_id   INTEGER NOT NULL,
    requested_at     INTEGER NOT NULL DEFAULT 0,
    latest_version   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, from_player_id)
);

CREATE TABLE user_friend_requests_outgoing (
    user_id          INTEGER NOT NULL REFERENCES users(user_id),
    to_player_id     INTEGER NOT NULL,
    requested_at     INTEGER NOT NULL DEFAULT 0,
    latest_version   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, to_player_id)
);

CREATE TABLE user_pvp_state (
    user_id            INTEGER NOT NULL PRIMARY KEY REFERENCES users(user_id),
    pvp_point          INTEGER NOT NULL DEFAULT 0,
    attack_win_count   INTEGER NOT NULL DEFAULT 0,
    attack_lose_count  INTEGER NOT NULL DEFAULT 0,
    defense_win_count  INTEGER NOT NULL DEFAULT 0,
    defense_lose_count INTEGER NOT NULL DEFAULT 0,
    last_finish_day    INTEGER NOT NULL DEFAULT 0,
    latest_version     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE user_pvp_logs (
    user_id             INTEGER NOT NULL REFERENCES users(user_id),
    is_defense          INTEGER NOT NULL,           -- 0 = attack log, 1 = defense log
    seq                 INTEGER NOT NULL,
    opponent_player_id  INTEGER NOT NULL,
    opponent_name       TEXT    NOT NULL DEFAULT '',
    opponent_pvp_point  INTEGER NOT NULL DEFAULT 0,
    opponent_deck_power INTEGER NOT NULL DEFAULT 0,
    is_victory          INTEGER NOT NULL DEFAULT 0,
    battle_datetime     INTEGER NOT NULL DEFAULT 0,
    fluctuated_point    INTEGER NOT NULL DEFAULT 0,
    rank                INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, is_defense, seq)
);

CREATE TABLE player_snapshots (
    player_id           INTEGER NOT NULL PRIMARY KEY,
    user_name           TEXT    NOT NULL DEFAULT '',
    level               INTEGER NOT NULL DEFAULT 0,
    max_deck_power      INTEGER NOT NULL DEFAULT 0,
    favorite_costume_id INTEGER NOT NULL DEFAULT 0,
    pvp_point           INTEGER NOT NULL DEFAULT 0,
    last_login_datetime INTEGER NOT NULL DEFAULT 0,
    defense_deck_json   TEXT    NOT NULL DEFAULT '[]',
    updated_at          INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_player_snapshots_pvp_point ON player_snapshots(pvp_point DESC);

-- +goose Down
DROP TABLE IF EXISTS player_snapshots;
DROP TABLE IF EXISTS user_pvp_logs;
DROP TABLE IF EXISTS user_pvp_state;
DROP TABLE IF EXISTS user_friend_requests_outgoing;
DROP TABLE IF EXISTS user_friend_requests_incoming;
DROP TABLE IF EXISTS user_friends;
