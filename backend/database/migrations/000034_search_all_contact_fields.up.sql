-- Recreate contacts_fts with every free-text column (FTS5 cannot add columns) and fill it.
-- The contact list still searches the first seven columns only (a filter in the query).

DROP TRIGGER IF EXISTS contacts_fts_ad;
DROP TRIGGER IF EXISTS contacts_fts_au;
DROP TRIGGER IF EXISTS contacts_fts_ai;
DROP TABLE IF EXISTS contacts_fts;

CREATE VIRTUAL TABLE contacts_fts USING fts5(
    firstname, lastname, nickname, email, phone, emails, phones, middle_name, organization, department, job_title, role, how_we_met, food_preference, work_information, contact_information, note, addresses, circles, custom_fields,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER contacts_fts_ai AFTER INSERT ON contacts BEGIN
    INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones, middle_name, organization, department, job_title, role, how_we_met, food_preference, work_information, contact_information, note, addresses, circles, custom_fields)
    VALUES (new.id,
        new.firstname, new.lastname, new.nickname, new.email, new.phone,
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.emails) THEN new.emails ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.phones) THEN new.phones ELSE '[]' END) AS j), ''),
        new.middle_name, new.organization, new.department, new.job_title, new.role,
        new.how_we_met, new.food_preference, new.work_information, new.contact_information, new.note,
        trim(COALESCE((SELECT group_concat(COALESCE(json_extract(j.value, '$.street'), '') || ' ' || COALESCE(json_extract(j.value, '$.city'), '') || ' ' || COALESCE(json_extract(j.value, '$.region'), '') || ' ' || COALESCE(json_extract(j.value, '$.postal'), '') || ' ' || COALESCE(json_extract(j.value, '$.country'), ''), ' ') FROM json_each(CASE WHEN json_valid(new.addresses) AND json_type(new.addresses) = 'array' THEN new.addresses ELSE '[]' END) AS j), '') || ' ' || COALESCE(new.address, '')),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(new.circles) AND json_type(new.circles) = 'array' THEN new.circles ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(new.custom_fields) AND json_type(new.custom_fields) = 'object' THEN new.custom_fields ELSE '{}' END) AS j), '')
    );
END;

CREATE TRIGGER contacts_fts_au AFTER UPDATE OF firstname, lastname, nickname, email, phone, emails, phones, middle_name, organization, department, job_title, role, how_we_met, food_preference, work_information, contact_information, note, circles, custom_fields, address, addresses ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
    INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones, middle_name, organization, department, job_title, role, how_we_met, food_preference, work_information, contact_information, note, addresses, circles, custom_fields)
    VALUES (new.id,
        new.firstname, new.lastname, new.nickname, new.email, new.phone,
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.emails) THEN new.emails ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(new.phones) THEN new.phones ELSE '[]' END) AS j), ''),
        new.middle_name, new.organization, new.department, new.job_title, new.role,
        new.how_we_met, new.food_preference, new.work_information, new.contact_information, new.note,
        trim(COALESCE((SELECT group_concat(COALESCE(json_extract(j.value, '$.street'), '') || ' ' || COALESCE(json_extract(j.value, '$.city'), '') || ' ' || COALESCE(json_extract(j.value, '$.region'), '') || ' ' || COALESCE(json_extract(j.value, '$.postal'), '') || ' ' || COALESCE(json_extract(j.value, '$.country'), ''), ' ') FROM json_each(CASE WHEN json_valid(new.addresses) AND json_type(new.addresses) = 'array' THEN new.addresses ELSE '[]' END) AS j), '') || ' ' || COALESCE(new.address, '')),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(new.circles) AND json_type(new.circles) = 'array' THEN new.circles ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(new.custom_fields) AND json_type(new.custom_fields) = 'object' THEN new.custom_fields ELSE '{}' END) AS j), '')
    );
END;

CREATE TRIGGER contacts_fts_ad AFTER DELETE ON contacts BEGIN
    DELETE FROM contacts_fts WHERE rowid = old.id;
END;

INSERT INTO contacts_fts(rowid, firstname, lastname, nickname, email, phone, emails, phones, middle_name, organization, department, job_title, role, how_we_met, food_preference, work_information, contact_information, note, addresses, circles, custom_fields)
SELECT c.id,
        c.firstname, c.lastname, c.nickname, c.email, c.phone,
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(c.emails) THEN c.emails ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(json_extract(j.value, '$.value'), ' ') FROM json_each(CASE WHEN json_valid(c.phones) THEN c.phones ELSE '[]' END) AS j), ''),
        c.middle_name, c.organization, c.department, c.job_title, c.role,
        c.how_we_met, c.food_preference, c.work_information, c.contact_information, c.note,
        trim(COALESCE((SELECT group_concat(COALESCE(json_extract(j.value, '$.street'), '') || ' ' || COALESCE(json_extract(j.value, '$.city'), '') || ' ' || COALESCE(json_extract(j.value, '$.region'), '') || ' ' || COALESCE(json_extract(j.value, '$.postal'), '') || ' ' || COALESCE(json_extract(j.value, '$.country'), ''), ' ') FROM json_each(CASE WHEN json_valid(c.addresses) AND json_type(c.addresses) = 'array' THEN c.addresses ELSE '[]' END) AS j), '') || ' ' || COALESCE(c.address, '')),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(c.circles) AND json_type(c.circles) = 'array' THEN c.circles ELSE '[]' END) AS j), ''),
        COALESCE((SELECT group_concat(j.value, ' ') FROM json_each(CASE WHEN json_valid(c.custom_fields) AND json_type(c.custom_fields) = 'object' THEN c.custom_fields ELSE '{}' END) AS j), '')
FROM contacts c;
