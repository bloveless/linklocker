CREATE TABLE tfa_application (
    id varchar(36) PRIMARY KEY NOT NULL,
    name varchar(256) NOT NULL UNIQUE,
    infobip_application_id varchar(32) NOT NULL
);

CREATE TABLE tfa_message (
    id varchar(36) PRIMARY KEY NOT NULL,
    application_id varchar(36) NOT NULL,
    name varchar(256) NOT NULL UNIQUE,
    infobip_message_id varchar(32) NOT NULL,

    FOREIGN KEY (application_id) REFERENCES tfa_application(id)
);

CREATE TABLE tfa_token (
    id varchar(36) PRIMARY KEY NOT NULL,
    user_id varchar(36) NOT NULL,
    token_type varchar(32) NOT NULL,
    token varchar(32) NOT NULL,
    delivery_method varchar(10) NOT NULL,
    created_at_utc varchar(19) NOT NULL,
    expires_at_utc varchar(19) NOT NULL,
    revoked integer NOT NULL DEFAULT(0),
    FOREIGN KEY (user_id) REFERENCES user(id)
);
