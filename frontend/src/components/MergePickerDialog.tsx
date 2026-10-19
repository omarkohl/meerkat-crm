import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Checkbox,
  DialogActions,
  DialogContent,
  DialogTitle,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  TextField,
  Typography,
} from '@mui/material';
import AppDialog from './AppDialog';
import { useContacts } from '../hooks/useContacts';
import { useDebouncedValue } from '../hooks/useDebounce';

interface MergePickerDialogProps {
  open: boolean;
  onClose: () => void;
  contact: { ID: number; firstname: string; lastname: string };
  onPicked: (ids: number[]) => void;
}

const name = (c: { firstname: string; lastname: string }) => [c.firstname, c.lastname].filter(Boolean).join(' ');

export default function MergePickerDialog({ open, onClose, contact, onPicked }: MergePickerDialogProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [picked, setPicked] = useState<number[]>([]);
  const search = useDebouncedValue(query.trim(), 300);
  const { contacts } = useContacts({ search, limit: 20, sort: 'firstname', order: 'asc', includeArchived: true });
  const others = contacts.filter((c) => c.ID !== contact.ID);

  useEffect(() => {
    if (open) {
      setQuery('');
      setPicked([]);
    }
  }, [open]);

  const toggle = (id: number) => {
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));
  };

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('merge.pickTitle')}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('merge.pickHint', { name: name(contact) })}
        </Typography>
        <TextField
          fullWidth
          size="small"
          label={t('merge.searchLabel')}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
          }}
        />
        <List dense>
          {others.length === 0 && (
            <ListItem>
              <ListItemText secondary={t('merge.noResults')} />
            </ListItem>
          )}
          {others.map((c) => (
            <ListItem key={c.ID} disablePadding>
              <ListItemButton
                onClick={() => {
                  toggle(c.ID);
                }}
              >
                <Checkbox
                  edge="start"
                  checked={picked.includes(c.ID)}
                  tabIndex={-1}
                  disableRipple
                  slotProps={{ input: { 'aria-label': t('merge.selectContact', { name: name(c) }) } }}
                />
                <ListItemText primary={name(c)} />
              </ListItemButton>
            </ListItem>
          ))}
        </List>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('common.cancel')}</Button>
        <Button
          variant="contained"
          disabled={picked.length === 0}
          onClick={() => {
            onPicked([contact.ID, ...picked]);
          }}
        >
          {t('common.continue')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
