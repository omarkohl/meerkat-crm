---
title: '0019. Images: storage and formats'
parent: ADRs
nav_order: 19
---

# 0019. Images: storage and formats

- Status: Accepted
- Date: 2026-10-22

## Context

Contacts, activities and notes need 0..n pictures (issues #111, #112). A contact keeps its single
profile photo in `contacts.photo`, which CardDAV, exports and lists read. Phone photos are large,
HEIC does not show in most browsers, and the avatar must stay a JPEG for iOS.

## Decision

- **Table `images`** with three nullable foreign keys (`contact_id`, `activity_id`, `note_id`, at
  most one set; none means pending). SQLite enforces ownership and cascades. Rows are deleted for
  good. `contacts.main_image_id` points at the main image.
- **Files** per user: `<PROFILE_PHOTO_DIR>/images/u<user_id>/<uuid>_image.webp` and `_thumb.webp`.
  The full variant has a long edge of at most 2048 px (never enlarged). The thumbnail has 480 px. No
  original is kept.
- **WebP** through `github.com/gen2brain/webp` (WASM, no cgo): lossy quality 82, lossless for PNG
  sources and images with transparency; thumbnails lossy quality 75. Go has no other WebP encoder.
  The library cannot write EXIF, so `imageproc` adds the `EXIF` chunk and `VP8X` header itself.
- **Metadata**: pixels are rotated by the EXIF orientation, then the block is stored with the
  orientation set to 1. Thumbnails have none. The capture time is stored in `taken_at`.
- **Derived avatar**: `contacts.photo` and its thumbnail stay and are made from the main image and
  its crop (photo at most 400 px, thumbnail 96 px), so CardDAV, exports and lists need no change.
  `contacts.main_image_id` and the crop in `images.crop_*` are the source. At startup a background
  job makes one image from each existing profile picture (idempotent); the avatar is left as is.
- **One decoder**: `backend/imageproc` reads JPEG, PNG, GIF (first frame), WebP and HEIC, with a 100
  MP limit and at most two decodes at the same time.
- **Clean-up**: the database is the truth. A daily sweep (in the trash purge slot) deletes pending
  images older than 24 hours and files no row refers to that are older than 24 hours.

## Consequences

- Spike result (desktop CPU, 1536x2048 noisy image): lossy encode 0.43 s, lossless 1.4 s, 140 to 400
  MB of memory. Real screenshots encode much faster. The format is stored per image (`images.mime`),
  so a later switch to JPEG or PNG needs no migration.
- The Go WebP decoder returns 4:2:0 chroma even for lossless files. Browsers decode them exactly.
  Only server-side crops (the avatar) see the subsampling, which a JPEG avatar has anyway.
- A full image is at most about 2 MB. Backups grow with the number of images.
