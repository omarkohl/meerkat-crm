import { useState } from 'react';
import { createNote } from '../api/notes';
import { createActivity, updateActivity, Activity } from '../api/activities';
import { endDateToSend } from '../utils/activityDates';
import { handleError, ErrorNotifier, getErrorMessage } from '../utils/errorHandler';

export function useContactDialogs(
  contactId: string | undefined,
  onRefresh: () => Promise<void>,
  notifier?: ErrorNotifier,
) {
  const [noteDialogOpen, setNoteDialogOpen] = useState(false);
  const [activityDialogOpen, setActivityDialogOpen] = useState(false);

  const handleSaveNote = async (content: string, date: string, imageIds: number[]) => {
    if (!contactId) return;

    try {
      await createNote(contactId, {
        content,
        date: new Date(date).toISOString(),
        contact_id: parseInt(contactId),
        image_ids: imageIds,
      });
      await onRefresh();
    } catch (err) {
      handleError(err, { operation: 'saving note' }, notifier);
      throw new Error(getErrorMessage(err));
    }
  };

  const handleSaveActivity = async (activity: {
    title: string;
    description: string;
    location: string;
    date: string;
    end_date?: string;
    contact_ids: number[];
    image_ids: number[];
  }) => {
    try {
      await createActivity({
        title: activity.title,
        description: activity.description,
        location: activity.location,
        date: new Date(activity.date).toISOString(),
        end_date: endDateToSend(activity.date, activity.end_date),
        contact_ids: activity.contact_ids,
        image_ids: activity.image_ids,
      });
      await onRefresh();
    } catch (err) {
      handleError(err, { operation: 'saving activity' }, notifier);
      throw new Error(getErrorMessage(err));
    }
  };

  const handleAddToActivities = async (activities: Activity[]) => {
    if (!contactId) return;

    try {
      for (const activity of activities) {
        const contactIds = (activity.contacts ?? []).map((c) => c.ID);
        await updateActivity(activity.ID, {
          title: activity.title,
          description: activity.description,
          location: activity.location,
          date: activity.date,
          end_date: activity.end_date,
          contact_ids: [...contactIds, parseInt(contactId)],
        });
      }
    } catch (err) {
      handleError(err, { operation: 'adding contact to activities' }, notifier);
      throw new Error(getErrorMessage(err));
    } finally {
      await onRefresh();
    }
  };

  return {
    noteDialogOpen,
    activityDialogOpen,
    setNoteDialogOpen,
    setActivityDialogOpen,
    handleSaveNote,
    handleSaveActivity,
    handleAddToActivities,
  };
}
