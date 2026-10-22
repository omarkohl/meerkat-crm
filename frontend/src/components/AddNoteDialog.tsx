import { useState } from 'react';
import { DialogTitle, DialogContent, DialogActions, TextField, Button, Box, Typography } from '@mui/material';
import AppDialog from './AppDialog';
import DateField from './DateField';
import PhotoDateHint from './images/PhotoDateHint';
import { useTranslation } from 'react-i18next';
import { now } from '../utils/now';
import { useImageUploads } from '../hooks/useImageUploads';
import ImageAttachmentField from './images/ImageAttachmentField';

interface AddNoteDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (content: string, date: string, imageIds: number[]) => Promise<void>;
}

export default function AddNoteDialog({ open, onClose, onSave }: AddNoteDialogProps) {
  const { t } = useTranslation();
  const [content, setContent] = useState('');
  const [date, setDate] = useState(now().toISOString().slice(0, 10));
  const [dateTouched, setDateTouched] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const uploads = useImageUploads();

  const handleSave = async () => {
    if (!content.trim() && uploads.imageIds.length === 0) {
      setError(t('noteDialog.requiredOrImage'));
      return;
    }

    setSaving(true);
    try {
      await onSave(content, date, uploads.imageIds);
      handleClose();
    } catch {
      setError(t('noteDialog.saveError'));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setContent('');
    setDate(now().toISOString().slice(0, 10));
    setDateTouched(false);
    setError('');
    uploads.reset();
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('noteDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label={t('noteDialog.content')}
            placeholder={t('noteDialog.contentPlaceholder')}
            multiline
            rows={4}
            value={content}
            onChange={(e) => {
              setContent(e.target.value);
              setError('');
            }}
            error={!!error}
            helperText={error}
            fullWidth
            autoFocus
          />
          <ImageAttachmentField uploads={uploads} disabled={saving} />
          <DateField
            label={t('noteDialog.date')}
            value={date}
            onChange={(next) => {
              setDate(next);
              setDateTouched(true);
            }}
            fullWidth
          />
          <PhotoDateHint takenAt={uploads.items[0]?.takenAt} date={date} touched={dateTouched} onUse={setDate} />
        </Box>
      </DialogContent>
      <DialogActions>
        {uploads.busy && (
          <Typography variant="caption" color="text.secondary" role="status" sx={{ mr: 'auto', ml: 1 }}>
            {t('images.uploading', { count: uploads.items.filter((i) => i.status === 'uploading').length })}
          </Typography>
        )}
        <Button onClick={handleClose} disabled={saving}>
          {t('noteDialog.cancel')}
        </Button>
        <Button onClick={handleSave} variant="contained" disabled={saving || uploads.busy}>
          {t('noteDialog.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
