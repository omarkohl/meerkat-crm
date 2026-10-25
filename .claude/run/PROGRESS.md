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

- The per-contact vCard filename is a slug of the name with ASCII letters and digits only, so a name
  in another script (for example "Zoë" or Cyrillic) loses those characters or falls back to
  `contact.vcf` (same as upstream). Keep it? what do you suggest?

## Blocked
