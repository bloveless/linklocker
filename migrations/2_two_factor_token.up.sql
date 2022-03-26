CREATE TABLE two_factor_token (
    id varchar(36) PRIMARY KEY NOT NULL,
    user_id varchar(36) NOT NULL,
    token_type varchar(32) NOT NULL,
    token varchar(32) NOT NULL,
    expires_at_utc varchar(19) NOT NULL,
    revoked integer NOT NULL DEFAULT(0),
    FOREIGN KEY (user_id) REFERENCES user(id)
);
