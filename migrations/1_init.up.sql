CREATE TABLE user (
    id varchar(36) PRIMARY KEY NOT NULL,
    email varchar(255) NOT NULL UNIQUE,
    password varchar(97) NOT NULL,
    name varchar(100) NOT NULL,
    phone_number varchar(20) NOT NULL
);

CREATE TABLE link (
    id varchar(36) PRIMARY KEY NOT NULL,
    user_id varchar(36) NOT NULL,
    sort_order int NOT NULL,
    url varchar(1000) NOT NULL,
    display_url varchar(1000) NULL,
    screenshot_url varchar(1000) NULL,
    FOREIGN KEY (user_id) REFERENCES user(id)
);
