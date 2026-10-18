import { useState } from 'react';
import { updateNote, deleteNote, Note } from '../api/notes';
import { updateActivity, deleteActivity, Activity } from '../api/activities';
import { handleError, ErrorNotifier } from '../utils/errorHandler';
import { now } from '../utils/now';

export function useTimelineEditing(
  contactId: number | undefined,
  onRefresh: () => Promise<void>,
  notifier?: ErrorNotifier,
) {
  const [editingTimelineItem, setEditingTimelineItem] = useState<{ type: 'note' | 'activity'; id: number } | null>(
    null,
  );
  const [editTimelineValues, setEditTimelineValues] = useState<{
    noteContent?: string;
    noteDate?: string;
    activityTitle?: string;
    activityDescription?: string;
    activityLocation?: string;
    activityDate?: string;
    activityContacts?: { ID: number; firstname: string; lastname: string; nickname?: string }[];
  }>({});

  const handleStartEditTimelineItem = (type: 'note' | 'activity', item: Note | Activity) => {
    setEditingTimelineItem({ type, id: item.ID });

    if (type === 'note') {
      const note = item as Note;
      setEditTimelineValues({
        noteContent: note.content || '',
        noteDate: note.date ? new Date(note.date).toISOString().split('T')[0] : '',
      });
    } else {
      const activity = item as Activity;

      setEditTimelineValues({
        activityTitle: activity.title || '',
        activityDescription: activity.description || '',
        activityLocation: activity.location || '',
        activityDate: activity.date ? new Date(activity.date).toISOString().split('T')[0] : '',
        activityContacts: activity.contacts || [],
      });
    }
  };

  const handleCancelEditTimelineItem = () => {
    setEditingTimelineItem(null);
    setEditTimelineValues({});
  };

  const handleUpdateNote = async (noteId: number) => {
    if (!editTimelineValues.noteContent?.trim()) return;

    try {
      await updateNote(noteId, {
        content: editTimelineValues.noteContent,
        date: editTimelineValues.noteDate ? new Date(editTimelineValues.noteDate).toISOString() : now().toISOString(),
        contact_id: contactId,
      });
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      handleError(err, { operation: 'updating note' }, notifier);
    }
  };

  const handleUpdateActivity = async (activityId: number) => {
    if (!editTimelineValues.activityTitle?.trim()) return;

    try {
      await updateActivity(activityId, {
        title: editTimelineValues.activityTitle,
        description: editTimelineValues.activityDescription || '',
        location: editTimelineValues.activityLocation || '',
        date: editTimelineValues.activityDate
          ? new Date(editTimelineValues.activityDate).toISOString()
          : now().toISOString(),
        contact_ids: editTimelineValues.activityContacts?.map((c) => c.ID) || [],
      });
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      handleError(err, { operation: 'updating activity' }, notifier);
    }
  };

  const handleDeleteNote = async (noteId: number) => {
    try {
      await deleteNote(noteId);
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      handleError(err, { operation: 'deleting note' }, notifier);
    }
  };

  const handleDeleteActivity = async (activityId: number) => {
    try {
      await deleteActivity(activityId);
      await onRefresh();
      handleCancelEditTimelineItem();
    } catch (err) {
      handleError(err, { operation: 'deleting activity' }, notifier);
    }
  };

  return {
    editingTimelineItem,
    editTimelineValues,
    handleStartEditTimelineItem,
    handleCancelEditTimelineItem,
    handleUpdateNote,
    handleUpdateActivity,
    handleDeleteNote,
    handleDeleteActivity,
    setEditTimelineValues,
  };
}
