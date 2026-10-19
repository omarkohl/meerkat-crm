// Merging contacts is done with the ordinary endpoints, one step after the other.
import { getContact, updateContact, deleteContact, getContactProfilePicture, uploadProfilePicture } from './contacts';
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

export async function mergeContacts(plan: MergePlan, choices: Record<string, string>): Promise<void> {
  const targetId = plan.target.ID;
  const sourceIds = plan.sources.map((s) => s.ID);

  await updateContact(targetId, buildMergedInput(plan, choices));

  const photoSource = plan.target.photo ? undefined : plan.sources.find((s) => s.photo);
  if (photoSource) {
    const photo = await getContactProfilePicture(photoSource.ID);
    if (photo) await uploadProfilePicture(targetId, photo);
  }

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
