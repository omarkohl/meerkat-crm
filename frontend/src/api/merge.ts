// Merging contacts is done with the ordinary endpoints, one step after the other.
import {
  getContact,
  getContactProfilePicture,
  updateContact,
  deleteContact,
  setContactImages,
  setMainImage,
} from './contacts';
import { uploadImage, type ImageRef } from './images';
import type { ContactResponse } from './contacts';
import { updateNote } from './notes';
import { updateReminder, type ReminderFormData } from './reminders';
import { getActivity, updateActivity } from './activities';
import {
  createRelationship,
  deleteRelationship,
  getIncomingRelationships,
  getRelationships,
  updateRelationship,
} from './relationships';
import { buildMergedInput, type MergePlan } from '../utils/mergeContacts';

export function getFullContact(id: number): Promise<ContactResponse> {
  return getContact(id) as Promise<ContactResponse>;
}

// A contact whose photo is not yet migrated to an image has only the avatar; upload it as an image
// so that it neither blocks nor loses to the photos of the others.
async function legacyPhoto(contact: MergePlan['target']): Promise<ImageRef[]> {
  if ((contact.images ?? []).length > 0 || !contact.photo) return [];
  const blob = await getContactProfilePicture(contact.ID);
  if (!blob) return [];
  return [await uploadImage(new File([blob], 'photo', { type: blob.type }), () => undefined).promise];
}

async function mergeImages({ target, sources }: MergePlan): Promise<void> {
  const own = [...(target.images ?? []), ...(await legacyPhoto(target))];
  const incoming = [...sources.flatMap((s) => s.images ?? [])];
  for (const source of sources) incoming.push(...(await legacyPhoto(source)));
  if (incoming.length === 0) return;

  const donor = own.length === 0 ? sources.find((s) => s.main_image_id != null) : undefined;
  const main = incoming.find((i) => i.id === donor?.main_image_id);
  const rest = incoming.filter((i) => i !== main);
  const ids = [...own, ...(main ? [main] : []), ...rest].map((i) => i.id);
  await setContactImages(target.ID, ids);
  if (main?.crop) await setMainImage(target.ID, main.id, main.crop);
}

export async function mergeContacts(plan: MergePlan, choices: Record<string, string>): Promise<void> {
  const targetId = plan.target.ID;
  const sourceIds = plan.sources.map((s) => s.ID);

  await updateContact(targetId, buildMergedInput(plan, choices));

  await mergeImages(plan);

  for (const id of plan.moves.activityIds) {
    const activity = await getActivity(id);
    const contactIds = (activity.contacts ?? []).map((c) => c.ID).filter((cid) => !sourceIds.includes(cid));
    await updateActivity(id, {
      title: activity.title,
      description: activity.description,
      location: activity.location,
      date: activity.date,
      contact_ids: [...new Set([...contactIds, targetId])],
    });
  }
  for (const note of plan.moves.notes) {
    await updateNote(note.ID, { content: note.content, date: note.date, contact_id: targetId });
  }
  for (const reminder of plan.moves.reminders) {
    await updateReminder(reminder.ID, {
      message: reminder.message,
      remind_at: reminder.remind_at,
      recurrence: reminder.recurrence as ReminderFormData['recurrence'],
      contact_id: targetId,
    });
  }

  for (const r of plan.moves.selfRelationships) {
    await deleteRelationship(targetId, r.ID);
  }
  for (const r of plan.moves.relationships) {
    await createRelationship(targetId, {
      name: r.name,
      type: r.type,
      gender: r.gender,
      birthday: r.birthday,
      related_contact_id: r.related_contact_id,
    });
  }
  for (const sourceId of sourceIds) {
    const { incoming_relationships } = await getIncomingRelationships(sourceId);
    for (const r of incoming_relationships) {
      if (r.contact_id === targetId || sourceIds.includes(r.contact_id)) continue;
      const owned = (await getRelationships(r.contact_id)).relationships;
      if (owned.some((o) => o.related_contact_id === targetId && o.type === r.type)) {
        await deleteRelationship(r.contact_id, r.ID);
        continue;
      }
      await updateRelationship(r.contact_id, r.ID, {
        name: r.name,
        type: r.type,
        gender: r.gender,
        birthday: r.birthday,
        related_contact_id: targetId,
      });
    }
  }

  for (const id of sourceIds) {
    await deleteContact(id);
  }
}
