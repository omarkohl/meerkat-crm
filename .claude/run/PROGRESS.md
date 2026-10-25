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
  (no e-mail channel, queue failure): nothing is marked then. Mark them separately?

- Upstream's VERSION:2.1 gate (ported as-is) no longer normalizes quoted-printable or non-UTF-8
  CHARSET values in files that declare 3.0/4.0 or no VERSION. Old behavior converted them. Keep the
  gate?

## Blocked
