import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Button,
  Box,
  Autocomplete,
  Chip,
  IconButton,
  CircularProgress,
} from '@mui/material';
import AppDialog from './AppDialog';
import DateField from './DateField';
import ActivityDateFields from './ActivityDateFields';
import DeleteIcon from '@mui/icons-material/Delete';
import { Contact, getContacts } from '../api/contacts';
import { endsBeforeStart } from '../utils/activityDates';

export interface TimelineItemValues {
  noteContent?: string;
  noteDate?: string;
  activityTitle?: string;
  activityDescription?: string;
  activityLocation?: string;
  activityDate?: string;
  activityEndDate?: string;
  activityContacts?: { ID: number; firstname: string; lastname: string; nickname?: string }[];
}

interface EditTimelineItemDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: () => void;
  onDelete: () => void;
  type: 'note' | 'activity';
  values: TimelineItemValues;
  onChange: (values: TimelineItemValues) => void;
}

export default function EditTimelineItemDialog({
  open,
  onClose,
  onSave,
  onDelete,
  type,
  values,
  onChange,
}: EditTimelineItemDialogProps) {
  const { t } = useTranslation();
  const [contactOptions, setContactOptions] = useState<Contact[]>([]);
  const [searchInput, setSearchInput] = useState('');
  const [loadingContacts, setLoadingContacts] = useState(false);

  useEffect(() => {
    if (!open || type !== 'activity') return;
    let stale = false;
    const timeoutId = setTimeout(() => {
      setLoadingContacts(true);
      getContacts({ limit: 100, search: searchInput })
        .then((response) => {
          if (!stale) setContactOptions(response.contacts);
        })
        .catch((err: unknown) => {
          console.error('Failed to fetch contacts:', err);
        })
        .finally(() => {
          if (!stale) setLoadingContacts(false);
        });
    }, 300);
    return () => {
      stale = true;
      clearTimeout(timeoutId);
    };
  }, [open, type, searchInput]);

  const handleSave = () => {
    onSave();
  };

  const handleDelete = () => {
    if (window.confirm(t('contactDetail.confirmDelete'))) {
      onDelete();
    }
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{type === 'note' ? t('contactDetail.editNote') : t('contactDetail.editActivity')}</DialogTitle>
      <DialogContent>
        {type === 'note' ? (
          // Edit Note
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
            <TextField
              fullWidth
              multiline
              rows={6}
              value={values.noteContent || ''}
              onChange={(e) => {
                onChange({ ...values, noteContent: e.target.value });
              }}
              label={t('noteDialog.content')}
              placeholder={t('noteDialog.contentPlaceholder')}
              autoFocus
            />
            <DateField
              fullWidth
              value={values.noteDate || ''}
              onChange={(noteDate) => {
                onChange({ ...values, noteDate });
              }}
              label={t('noteDialog.date')}
            />
          </Box>
        ) : (
          // Edit Activity
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
            <TextField
              fullWidth
              value={values.activityTitle || ''}
              onChange={(e) => {
                onChange({ ...values, activityTitle: e.target.value });
              }}
              label={t('activityDialog.activityTitle')}
              autoFocus
            />
            <TextField
              fullWidth
              multiline
              rows={3}
              value={values.activityDescription || ''}
              onChange={(e) => {
                onChange({ ...values, activityDescription: e.target.value });
              }}
              label={t('activityDialog.description')}
            />
            <TextField
              fullWidth
              value={values.activityLocation || ''}
              onChange={(e) => {
                onChange({ ...values, activityLocation: e.target.value });
              }}
              label={t('activityDialog.location')}
            />
            <ActivityDateFields
              date={values.activityDate || ''}
              endDate={values.activityEndDate || ''}
              onChange={(next) => {
                onChange({ ...values, activityDate: next.date, activityEndDate: next.endDate });
              }}
            />
            <Autocomplete
              multiple
              options={contactOptions}
              getOptionLabel={(contact) =>
                `${contact.firstname}${contact.nickname ? ` "${contact.nickname}"` : ''} ${contact.lastname}`
              }
              value={values.activityContacts || []}
              onChange={(_, newValue) => {
                onChange({ ...values, activityContacts: newValue });
              }}
              onInputChange={(_, value) => {
                setSearchInput(value);
              }}
              inputValue={searchInput}
              loading={loadingContacts}
              filterOptions={(x) => x}
              isOptionEqualToValue={(option, value) => option.ID === value.ID}
              renderInput={(params) => (
                <TextField
                  {...params}
                  label={t('activityDialog.contacts')}
                  placeholder={t('activityDialog.selectContacts')}
                  slotProps={{
                    input: {
                      ...params.InputProps,
                      endAdornment: (
                        <>
                          {loadingContacts ? <CircularProgress color="inherit" size={20} /> : null}
                          {params.InputProps.endAdornment}
                        </>
                      ),
                    },
                  }}
                />
              )}
              renderTags={(value, getTagProps) =>
                value.map((contact, index) => (
                  <Chip
                    label={`${contact.firstname}${contact.nickname ? ` "${contact.nickname}"` : ''} ${contact.lastname}`}
                    {...getTagProps({ index })}
                    size="small"
                    key={contact.ID}
                  />
                ))
              }
            />
          </Box>
        )}
      </DialogContent>
      <DialogActions sx={{ justifyContent: 'space-between', px: 3, pb: 2 }}>
        <IconButton color="error" onClick={handleDelete} title={t('contactDetail.delete')}>
          <DeleteIcon />
        </IconButton>
        <Box>
          <Button onClick={onClose} sx={{ mr: 1 }}>
            {t('contactDetail.cancel')}
          </Button>
          <Button
            onClick={handleSave}
            variant="contained"
            disabled={type === 'activity' && endsBeforeStart(values.activityDate ?? '', values.activityEndDate)}
          >
            {t('contactDetail.save')}
          </Button>
        </Box>
      </DialogActions>
    </AppDialog>
  );
}
