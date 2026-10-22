import { useState, useEffect, useCallback } from 'react';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  Typography,
  Autocomplete,
  Chip,
  CircularProgress,
  ToggleButton,
  ToggleButtonGroup,
} from '@mui/material';
import AppDialog from './AppDialog';
import ActivityDateFields from './ActivityDateFields';
import PhotoDateHint from './images/PhotoDateHint';
import { endsBeforeStart } from '../utils/activityDates';
import { useTranslation } from 'react-i18next';
import { Contact, getContacts } from '../api/contacts';
import { Activity } from '../api/activities';
import ExistingActivitiesPicker from './ExistingActivitiesPicker';
import { now } from '../utils/now';
import { useImageUploads } from '../hooks/useImageUploads';
import ImageAttachmentField from './images/ImageAttachmentField';

interface AddActivityDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (activity: {
    title: string;
    description: string;
    location: string;
    date: string;
    end_date?: string;
    contact_ids: number[];
    image_ids: number[];
  }) => Promise<void>;
  onAddToActivities?: (activities: Activity[]) => Promise<void>;
  preselectedContact?: Contact;
}

export default function AddActivityDialog({
  open,
  onClose,
  onSave,
  onAddToActivities,
  preselectedContact,
}: AddActivityDialogProps) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<'new' | 'existing'>('new');
  const [existingActivities, setExistingActivities] = useState<Activity[]>([]);
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [location, setLocation] = useState('');
  const [date, setDate] = useState(now().toISOString().slice(0, 10));
  const [dateTouched, setDateTouched] = useState(false);
  const [endDate, setEndDate] = useState('');
  const [selectedContacts, setSelectedContacts] = useState<Contact[]>([]);
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [searchInput, setSearchInput] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);
  const uploads = useImageUploads();

  const loadContacts = useCallback(async (search: string = '') => {
    setLoading(true);
    try {
      const response = await getContacts({ limit: 100, search });
      setContacts(response.contacts || []);
    } catch (err) {
      console.error('Failed to fetch contacts:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  // Use the contact as given: it may be archived or beyond the first page of the contact list
  useEffect(() => {
    if (open && preselectedContact) {
      setSelectedContacts([preselectedContact]);
    }
  }, [open, preselectedContact]);

  // Debounced search effect
  useEffect(() => {
    if (!open) return;

    const timeoutId = setTimeout(() => {
      loadContacts(searchInput);
    }, 300);

    return () => {
      clearTimeout(timeoutId);
    };
  }, [searchInput, open, loadContacts]);

  const handleSave = async () => {
    if (mode === 'existing') {
      if (!onAddToActivities || existingActivities.length === 0) return;
      setSaving(true);
      try {
        await onAddToActivities(existingActivities);
        handleClose();
      } catch {
        // The notifier already showed the error; the dialog stays open to retry.
      } finally {
        setSaving(false);
      }
      return;
    }

    if (!title.trim() || !date) {
      setError(t('activityDialog.required'));
      return;
    }
    if (endsBeforeStart(date, endDate)) return;

    setSaving(true);
    try {
      await onSave({
        title,
        description,
        location,
        date,
        end_date: endDate || undefined,
        contact_ids: selectedContacts.map((c) => c.ID),
        image_ids: uploads.imageIds,
      });
      handleClose();
    } catch {
      setError('Failed to save activity');
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setMode('new');
    setExistingActivities([]);
    setTitle('');
    setDescription('');
    setLocation('');
    setDate(now().toISOString().slice(0, 10));
    setDateTouched(false);
    setEndDate('');
    setSelectedContacts([]);
    setSearchInput('');
    setContacts([]);
    setError('');
    uploads.reset();
    onClose();
  };

  const getContactLabel = (contact: Contact) => {
    return `${contact.firstname}${contact.nickname ? ` "${contact.nickname}"` : ''} ${contact.lastname}`;
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('activityDialog.title')}</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 1, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {preselectedContact && onAddToActivities && (
            <ToggleButtonGroup
              exclusive
              fullWidth
              size="small"
              value={mode}
              onChange={(_, value: 'new' | 'existing' | null) => {
                if (value) setMode(value);
              }}
            >
              <ToggleButton value="new">{t('activityDialog.newActivity')}</ToggleButton>
              <ToggleButton value="existing">{t('activityDialog.existingActivities')}</ToggleButton>
            </ToggleButtonGroup>
          )}

          {mode === 'existing' && preselectedContact ? (
            <ExistingActivitiesPicker
              contactId={preselectedContact.ID}
              value={existingActivities}
              onChange={setExistingActivities}
            />
          ) : (
            <>
              <TextField
                label={t('activityDialog.activityTitle')}
                placeholder={t('activityDialog.titlePlaceholder')}
                value={title}
                onChange={(e) => {
                  setTitle(e.target.value);
                  setError('');
                }}
                error={!!error}
                helperText={error}
                fullWidth
                required
                autoFocus
              />

              <TextField
                label={t('activityDialog.description')}
                placeholder={t('activityDialog.descriptionPlaceholder')}
                multiline
                rows={3}
                value={description}
                onChange={(e) => {
                  setDescription(e.target.value);
                }}
                fullWidth
              />

              <TextField
                label={t('activityDialog.location')}
                placeholder={t('activityDialog.locationPlaceholder')}
                value={location}
                onChange={(e) => {
                  setLocation(e.target.value);
                }}
                fullWidth
              />

              <ActivityDateFields
                date={date}
                endDate={endDate}
                onChange={(next) => {
                  setDate(next.date);
                  setEndDate(next.endDate);
                  setDateTouched(true);
                }}
                required
              />
              <PhotoDateHint takenAt={uploads.items[0]?.takenAt} date={date} touched={dateTouched} onUse={setDate} />

              <Autocomplete
                multiple
                options={contacts}
                getOptionLabel={getContactLabel}
                value={selectedContacts}
                onChange={(_, newValue) => {
                  setSelectedContacts(newValue);
                }}
                onInputChange={(_, value) => {
                  setSearchInput(value);
                }}
                inputValue={searchInput}
                loading={loading}
                filterOptions={(x) => x}
                isOptionEqualToValue={(option, value) => option.ID === value.ID}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label={t('activityDialog.contacts')}
                    placeholder={t('activityDialog.selectContacts')}
                    InputProps={{
                      ...params.InputProps,
                      endAdornment: (
                        <>
                          {loading ? <CircularProgress color="inherit" size={20} /> : null}
                          {params.InputProps.endAdornment}
                        </>
                      ),
                    }}
                  />
                )}
                renderTags={(value, getTagProps) =>
                  value.map((contact, index) => (
                    <Chip label={getContactLabel(contact)} {...getTagProps({ index })} key={contact.ID} />
                  ))
                }
              />

              <ImageAttachmentField uploads={uploads} disabled={saving} />
            </>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        {mode === 'new' && uploads.busy && (
          <Typography variant="caption" color="text.secondary" role="status" sx={{ mr: 'auto', ml: 1 }}>
            {t('images.uploading', { count: uploads.items.filter((i) => i.status === 'uploading').length })}
          </Typography>
        )}
        <Button onClick={handleClose} disabled={saving}>
          {t('activityDialog.cancel')}
        </Button>
        <Button
          onClick={handleSave}
          variant="contained"
          disabled={
            saving || (mode === 'new' && uploads.busy) || (mode === 'existing' && existingActivities.length === 0)
          }
        >
          {t('activityDialog.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
