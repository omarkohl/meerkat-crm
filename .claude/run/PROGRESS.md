# Progress

## Next step

0.

## Stack

<!-- step: branch, PR number (base) -->

## Log

- Birthday mail sent mark: `users.birthday_mail_sent_on` (migration 35), set after the mail is
  queued; skips the birthdays for that day on later runs.

## Decisions

## For review

## Questions

- Birthday webhooks (`birthday.occurred`) still fire on every digest run while no mail can be queued
  (no e-mail channel, queue failure): nothing is marked then. Mark them separately? explain the
  implications

- Upstream's VERSION:2.1 gate (ported as-is) no longer normalizes quoted-printable or non-UTF-8
  CHARSET values in files that declare 3.0/4.0 or no VERSION. Old behavior converted them. Keep the
  gate? keep upstream since I have no files to test it with now

- After migration 000041 a former Monica "Deceased" custom field is hidden until the user enables
  the Deceased field in the settings (same as upstream). leave like upstream

- The per-contact vCard filename is a slug of the name with ASCII letters and digits only, so a name
  in another script (for example "Zoë" or Cyrillic) loses those characters or falls back to
  `contact.vcf` (same as upstream). Keep it? what do you suggest?

- `docs/development/contributing.md` "Reporting Issues" links the upstream issue tracker
  (`fbuchner/meerkat-crm`), not the fork's. Point it at the fork? point at the fork

## Blocked
