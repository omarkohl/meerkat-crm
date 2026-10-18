-- Back to the index of migration 29: names, e-mail addresses and phone numbers only.
DROP TRIGGER IF EXISTS contacts_fts_ad;
DROP TRIGGER IF EXISTS contacts_fts_au;
DROP TRIGGER IF EXISTS contacts_fts_ai;
DROP TABLE IF EXISTS contacts_fts;

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, emails, phones,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones)
    VALUES (
        new.id, new.firstname, new.lastname, new.nickname, new.email, new.phone,
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.emails) THEN new.emails ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.phones) THEN new.phones ELSE '[]' END) AS j), '')
    );
END;

CREATE TRIGGER contacts_fts_au AFTER UPDATE OF firstname, lastname, nickname, email, phone, emails, phones ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
    INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones)
    VALUES (
        new.id, new.firstname, new.lastname, new.nickname, new.email, new.phone,
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.emails) THEN new.emails ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.phones) THEN new.phones ELSE '[]' END) AS j), '')
    );
END;

CREATE TRIGGER contacts_fts_ad AFTER DELETE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
END;

INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones)
SELECT c.id, c.firstname, c.lastname, c.nickname, c.email, c.phone,
    COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(c.emails) THEN c.emails ELSE '[]' END) AS j), ''),
    COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(c.phones) THEN c.phones ELSE '[]' END) AS j), '')
FROM contacts c;
