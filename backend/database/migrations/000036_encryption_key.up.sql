-- A fixed text encrypted with ENCRYPTION_KEY: a wrong key is found at startup.
CREATE TABLE encryption_key_check (
    id    INTEGER PRIMARY KEY CHECK (id = 1),
    value TEXT    NOT NULL
);

-- Two-factor login now uses only ENCRYPTION_KEY; data from before it cannot be read any more.
UPDATE users SET totp_secret = '', totp_enabled = 0, totp_last_step = 0;
DELETE FROM recovery_codes;
