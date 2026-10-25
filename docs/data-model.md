---
title: Data Model
nav_order: 3
has_children: false
---

# Data Model

## Contacts

Contacts are the core of Meerkat CRM and represent people from your network. Use **circles** to
group your contacts (e.g. Business, Friends, Bowling Group).

You can create a birthday reminder when creating a contact, or switch it on or off later next to the
birthday. This simply adds a reminder for this contact's birthday; changing the birthday moves it.
Independently of that you will receive emails (if set up) for all birthdays anyway.

You can add **custom fields** through the settings menu. These will appear on every contact and can
hold any text value. When deleting a custom field the information will still be stored in the
database but not be visible. It will reappear when you add the same custom field again.

**Profile pictures** are cut from the main image of a contact: a square crop, stored as a file of at
most 400x400px with a 96x96px thumbnail. You upload pictures directly, paste or drop them, or enter
an image URL and have the backend pull it in from the web. Currently JPEG, PNG and HEIC up to 10 MB
are supported file formats. See [Images](#images).

You can mark a contact as **deceased**, with an optional date of death, once you turn the field on
in the settings. A deceased contact no longer appears in the upcoming birthdays, the birthday e-mail
or the calendar feed. Its reminders stay.

You can **archive** contacts to hide them from default search results and the dashboard. Archiving
permanently deletes all active reminders for that contact. Notes, activities and relationships are
preserved. Archived contacts are still synced via CardDAV (as often you might have phone contacts
that you want to keep but do not want them to show up in Meerkat CRM).

**Merging** duplicates: select two or more contacts in the contact list and choose **Merge**, or use
**Merge with…** on a contact. You pick the contact to keep and, for fields with different values,
the value to use or your own. E-mail addresses, phone numbers, addresses, circles, links, social
profiles, custom fields and the CardDAV note are combined, and activities, notes, reminders and
relationships move to the kept contact. The other contacts go to the trash. A merge cannot be
undone.

**Deleting** a contact moves it to the **Trash** (sidebar). The contact keeps its notes, reminders,
relationships, activity links and photo there, and **Restore** brings everything back. While in the
trash the contact is hidden from lists, search, birthdays, the network graph, the calendar feed and
reminders. **Delete permanently** removes one contact for good, **Empty trash** removes all of them,
and anything that has been in the trash for more than 30 days (`TRASH_RETENTION_DAYS`) is removed
automatically once a day. Activities themselves are never deleted with a contact. Contacts deleted
before the trash existed already lost their activity links and photo files, so restoring them shows
no activities and may show a missing photo. If the contact is synced with a CardDAV address book,
deleting it deletes the remote card on the next sync, and restoring it uploads the card again.

## Activities

Use Activities to document events or interactions with one or more contacts (eg.dinner, phone call
or meeting). Each activity has a title, date, and can optionally include an end date (for an
activity that spans several days, such as a trip), a description, location, and linked contacts. If
an activity has at least two linked contacts it appears in the network graph.

Activities appear on each linked contact's detail page in the **Timeline** tab, most recent
activities show first. An activity is placed by its start date. The date filter shows an activity
when any of its days is in the range.

The global **Activities** page lists every activity across all contacts and offers searching and a
date range filter. Searching (contacts, activities and unassigned notes) ignores case and accents
and matches the start of each word you type, so `zoe mül` finds `Zoë Müller`; it does not match the
middle of a word (`ller` does not find `Müller`). Two kinds of input are searched as plain
substrings instead, as before: text that is only digits and phone punctuation (`795550` finds
`+41795550101`), and Chinese, Japanese and Korean text, which has no spaces between words. Only the
contact list ranks by relevance (the page offers "Best match" while you search); notes and
activities stay newest first. Search terms are cut at 200 characters and 32 words.

The **search box in the header** searches contacts, notes and activities at once, from two
characters on. It shows the best contacts first, then notes and activities, each group with its
total and the matching text marked. It searches every text field of a contact, not only the name,
e-mail addresses and phone numbers: the note, addresses, circles, organization, job title, how you
met, custom field values and so on, and tells which field matched. The contact list's search box
searches every text field too. The pickers (merge, activities, relationships) search names, e-mail
addresses and phone numbers only. A contact matched by name ranks above one matched by anything
else. "Show all results" (or Enter) opens a results page with tabs for All, Contacts, Notes and
Activities, each with its count; a tab pages through its group, and a result opens the contact, or
the Notes or Activities page filtered by the term. Trashed contacts and their notes are not found.

## Notes

Notes let you capture free-text information. All notes attached to a specific contact appear in that
contact's timeline. Use them to record things you've learned about a person, conversation highlights
or anything you want to remember. The **Note** field on a contact is different: it is one free-text
value that maps to the vCard `NOTE` property. It is filled by VCF/CSV import and synced via CardDAV,
so it matches the note your phone or address book shows. Timeline notes are not synced. Unassigned
notes are not attached to any contact. They are accessible from the Notes page in the main
navigation and can be used like a personal journal.

## Images

A contact, an activity or a note can have images. A contact can have up to 50; one is the main image
and has a square face crop that makes the profile picture. An image has one owner at most; one with
no owner is pending (uploaded, not saved with anything yet) and is deleted after 24 hours. Each
image has a full variant (WebP, long edge at most 2048 px) and a thumbnail (480 px) under
`PROFILE_PHOTO_DIR/images/u<user_id>/`. Images of a note or activity are deleted with it. Images of
a contact stay with the contact in the trash and are deleted when it is purged. See
[ADR 0019](adr/0019-images-storage-and-formats.md).

## Reminders

Reminders help you stay on top of important dates and follow-ups. Reminders can be one-time or recur
on a schedule. Reminders are tied to a specific contact and appear on both the contact's detail page
and the dashboard. You can decide wether the reminder should reschule from the completion date (e.g.
for a catch-up) or from the original date (e.g. for an anniversary). The **Stay in Touch** button on
a contact's detail page opens a prefilled reminder creation dialog for a quarterly catch-up.

If you enable **Send email notification** on a reminder, you will receive an email when the reminder
is due. This requires a valid email address on your account and a configured email channel (Resend
or SMTP) on the server.

When a reminder is due, you can complete or skip it. The difference is that selecting **Complete**
creates a completion entry on the related contact's timeline while the **Skip** option directly
schedules the next reminder occurence (if there is one) without creating a timeline entry. Overdue
reminders remain visible until they are completed or skipped though they will not show up in the
reminder emails again.

## Relationships

Relationships let you map connections between contacts. You can select a suggested relationship or
enter your own title. Relationships can either link to an existing contact or you can enter details
manually without creating a dedicated contact. Relationships are shown both on the outgoing contact
("Sister: Eva") as well as on the incoming contact detail page ("Sister of Jon").
