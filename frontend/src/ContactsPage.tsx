import { useEffect, useState, useMemo, useRef, useCallback } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useContacts } from './hooks/useContacts';
import { useDebouncedValue } from './hooks/useDebounce';
import { getCircles, type Contact } from './api/contacts';
import { getCurrentUser } from './api/admin';
import { resolveEnabledFields, ContactFieldKey } from './contactFields';
import AddContactDialog from './components/AddContactDialog';
import ImportContactsDialog from './components/ImportContactsDialog';
import MergeContactsDialog from './components/MergeContactsDialog';
import {
  Box,
  Card,
  Avatar,
  Typography,
  Chip,
  Stack,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Pagination,
  Button,
  FormControlLabel,
  TextField,
  Switch,
  Checkbox,
} from '@mui/material';
import PersonAddIcon from '@mui/icons-material/PersonAdd';
import FileUploadIcon from '@mui/icons-material/FileUpload';
import MergeIcon from '@mui/icons-material/CallMerge';
import { ContactListSkeleton } from './components/LoadingSkeletons';
import { thumbnailSrc } from './utils/thumbnail';

export default function ContactsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const urlSearch = searchParams.get('search') || '';
  const [searchInput, setSearchInput] = useState(urlSearch);
  const searchQuery = useDebouncedValue(searchInput, 400).trim();
  const page = parseInt(searchParams.get('page') || '1', 10);
  const [selectedCircle, setSelectedCircle] = useState('');
  const [circles, setCircles] = useState<string[]>([]);
  const [sortOption, setSortOption] = useState(() => {
    return localStorage.getItem('contacts-sort-option') || 'id-desc';
  });
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [customFieldNames, setCustomFieldNames] = useState<string[]>([]);
  const [enabledFields, setEnabledFields] = useState<Set<ContactFieldKey>>(() => resolveEnabledFields(null));
  const [showArchived, setShowArchived] = useState(false);
  const [selected, setSelected] = useState<Map<number, Contact>>(new Map());
  const selectedIds = useMemo(() => Array.from(selected.keys()), [selected]);
  const [showSelectedOnly, setShowSelectedOnly] = useState(false);
  if (showSelectedOnly && selected.size === 0) setShowSelectedOnly(false);
  const [mergeOpen, setMergeOpen] = useState(false);
  const pageSize = 10;

  // While searching, the default order is the best match first (the server ranks when no
  // sort is sent); picking another order is still possible and is not remembered.
  const searching = searchQuery.trim() !== '';
  const [searchSortOption, setSearchSortOption] = useState('relevance');
  const activeSort = searching ? searchSortOption : sortOption;
  const [sortField, sortOrder] = activeSort.split('-');

  // A link to this page with a term (from the search) replaces the typed one
  useEffect(() => {
    setSearchInput(urlSearch);
  }, [urlSearch]);

  // Persist sort option to localStorage
  useEffect(() => {
    localStorage.setItem('contacts-sort-option', sortOption);
  }, [sortOption]);

  // Helper to update page in URL
  const setPage = useCallback(
    (newPage: number) => {
      setSearchParams((prev) => {
        const params = new URLSearchParams(prev);
        if (newPage === 1) {
          params.delete('page');
        } else {
          params.set('page', String(newPage));
        }
        return params;
      });
    },
    [setSearchParams],
  );

  // Memoize params to prevent infinite re-renders
  const contactParams = useMemo(
    () => ({
      page,
      limit: pageSize,
      search: searchQuery,
      searchAll: true,
      circle: selectedCircle,
      sort: sortField === 'relevance' ? undefined : sortField,
      order: sortField === 'relevance' ? undefined : sortOrder,
      includeArchived: showArchived,
    }),
    [page, searchQuery, selectedCircle, sortField, sortOrder, showArchived],
  );

  // Use custom hook for fetching contacts
  const { contacts, total: totalContacts, loading, refetch } = useContacts(contactParams);

  // Fetch circles for filter and custom field names
  useEffect(() => {
    const fetchData = async () => {
      try {
        const [circlesData, user] = await Promise.all([getCircles(), getCurrentUser()]);
        setCircles(Array.isArray(circlesData) ? circlesData : []);
        setCustomFieldNames(user.custom_field_names ?? []);
        setEnabledFields(resolveEnabledFields(user.enabled_contact_fields));
      } catch (err) {
        console.error('Error fetching circles or custom field names:', err);
      }
    };
    fetchData();
  }, []);

  // Reset to page 1 when search or filter changes (but not on initial mount)
  const prevFiltersRef = useRef({ searchQuery, selectedCircle });
  useEffect(() => {
    const prev = prevFiltersRef.current;
    if (prev.searchQuery !== searchQuery || prev.selectedCircle !== selectedCircle) {
      setPage(1);
      prevFiltersRef.current = { searchQuery, selectedCircle };
    }
  }, [searchQuery, selectedCircle, setPage]);

  // Filter contacts by selected circle
  // With backend pagination, contacts are already filtered
  const filteredContacts = showSelectedOnly ? Array.from(selected.values()) : contacts;

  const handleContactAdded = (contactId: number) => {
    navigate(`/contacts/${contactId}`);
  };

  const handleImportComplete = async () => {
    await refetch();
    // Also refresh circles
    try {
      const data = await getCircles();
      setCircles(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Error fetching circles:', err);
    }
  };

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Typography variant="h5" gutterBottom sx={{ mb: 2 }}>
        {t('contacts.title')}
      </Typography>
      <TextField
        size="small"
        fullWidth
        label={t('contacts.search')}
        value={searchInput}
        onChange={(e) => {
          setSearchInput(e.target.value);
        }}
        sx={{ mb: 1.5 }}
      />
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems="center">
        <FormControl sx={{ minWidth: 180 }} size="small">
          <InputLabel id="circle-select-label">{t('contacts.filterByCircle')}</InputLabel>
          <Select
            labelId="circle-select-label"
            value={selectedCircle}
            label={t('contacts.filterByCircle')}
            onChange={(e) => {
              setSelectedCircle(e.target.value);
            }}
          >
            <MenuItem value="">{t('contacts.allCircles')}</MenuItem>
            {circles.map((circle) => (
              <MenuItem key={circle} value={circle}>
                {circle}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl sx={{ minWidth: 150 }} size="small">
          <InputLabel id="sort-select-label">{t('contacts.sortBy')}</InputLabel>
          <Select
            labelId="sort-select-label"
            value={activeSort}
            label={t('contacts.sortBy')}
            onChange={(e) => {
              searching ? setSearchSortOption(e.target.value) : setSortOption(e.target.value);
            }}
          >
            {searching && <MenuItem value="relevance">{t('contacts.sort.relevance')}</MenuItem>}
            <MenuItem value="id-desc">{t('contacts.sort.recentlyAdded')}</MenuItem>
            <MenuItem value="id-asc">{t('contacts.sort.oldestFirst')}</MenuItem>
            <MenuItem value="firstname-asc">{t('contacts.sort.nameAZ')}</MenuItem>
            <MenuItem value="firstname-desc">{t('contacts.sort.nameZA')}</MenuItem>
            <MenuItem value="random-asc">{t('contacts.sort.random')}</MenuItem>
          </Select>
        </FormControl>
        <FormControlLabel
          control={
            <Switch
              checked={showArchived}
              onChange={(e) => {
                setShowArchived(e.target.checked);
              }}
              size="small"
            />
          }
          label={t('contacts.showArchived')}
          sx={{ ml: 0.5, whiteSpace: 'nowrap' }}
        />
        {selectedIds.length >= 1 && (
          <FormControlLabel
            control={
              <Switch
                checked={showSelectedOnly}
                onChange={(e) => {
                  setShowSelectedOnly(e.target.checked);
                }}
                size="small"
              />
            }
            label={t('merge.selectedOnly', { count: selectedIds.length })}
            sx={{ ml: 0.5, whiteSpace: 'nowrap' }}
          />
        )}
        {selectedIds.length >= 1 && (
          <Button
            variant="outlined"
            onClick={() => {
              setSelected(new Map());
            }}
            sx={{ whiteSpace: 'nowrap' }}
          >
            {t('merge.clearSelection', { count: selectedIds.length })}
          </Button>
        )}
        {selectedIds.length >= 2 && (
          <Button
            variant="contained"
            startIcon={<MergeIcon />}
            onClick={() => {
              setMergeOpen(true);
            }}
            sx={{ whiteSpace: 'nowrap' }}
          >
            {t('merge.button')} ({selectedIds.length})
          </Button>
        )}
        <Button
          variant="outlined"
          startIcon={<FileUploadIcon />}
          onClick={() => {
            setImportDialogOpen(true);
          }}
          sx={{ whiteSpace: 'nowrap' }}
        >
          {t('contacts.import.button', 'Import')}
        </Button>
        <Button
          variant="outlined"
          startIcon={<PersonAddIcon />}
          onClick={() => {
            setAddDialogOpen(true);
          }}
          sx={{ whiteSpace: 'nowrap' }}
        >
          {t('contacts.add.button')}
        </Button>
      </Stack>
      {!showSelectedOnly && (totalContacts > 0 || searchQuery || selectedCircle) && (
        <Box
          sx={{
            mb: 2,
            p: 1.5,
            bgcolor: 'action.hover',
            borderRadius: 1,
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            flexWrap: 'wrap',
          }}
        >
          <Typography variant="body2" sx={{ flexGrow: 1 }}>
            {searchQuery && selectedCircle
              ? t('contacts.filteredBySearchAndCircle', {
                  search: searchQuery,
                  circle: selectedCircle,
                  count: totalContacts,
                })
              : searchQuery
                ? t('contacts.filteredBySearch', { search: searchQuery, count: totalContacts })
                : selectedCircle
                  ? t('contacts.filteredMessage', {
                      count: filteredContacts.length,
                      total: totalContacts,
                      circle: selectedCircle,
                    })
                  : t('contacts.totalContacts', { count: totalContacts })}
          </Typography>
          {searchQuery && (
            <Chip
              label={`"${searchQuery}"`}
              size="small"
              onDelete={() => {
                setSearchInput('');
                setSearchParams((prev) => {
                  const params = new URLSearchParams(prev);
                  params.delete('search');
                  return params;
                });
              }}
            />
          )}
          {selectedCircle && (
            <Chip
              label={selectedCircle}
              size="small"
              onDelete={() => {
                setSelectedCircle('');
                setPage(1);
              }}
            />
          )}
        </Box>
      )}
      {loading && !showSelectedOnly ? (
        <ContactListSkeleton count={10} />
      ) : (
        <>
          <Stack spacing={2}>
            {filteredContacts.map((contact) => (
              <Card
                key={contact.ID}
                component={Link}
                to={`/contacts/${contact.ID}`}
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  p: 1.5,
                  cursor: 'pointer',
                  textDecoration: 'none',
                  color: 'inherit',
                  bgcolor: contact.archived ? 'action.disabledBackground' : undefined,
                  '&:hover': {
                    bgcolor: 'action.hover',
                  },
                }}
              >
                <Checkbox
                  checked={selected.has(contact.ID)}
                  onClick={(e) => {
                    e.stopPropagation();
                  }}
                  onChange={() => {
                    setSelected((prev) => {
                      const next = new Map(prev);
                      if (!next.delete(contact.ID)) next.set(contact.ID, contact);
                      return next;
                    });
                  }}
                  slotProps={{
                    input: {
                      'aria-label': t('merge.selectContact', {
                        name: [contact.firstname, contact.lastname].filter(Boolean).join(' '),
                      }),
                    },
                  }}
                />
                <Avatar
                  src={thumbnailSrc(contact.photo_thumbnail)}
                  sx={{ width: 48, height: 48, mr: 1.5, bgcolor: 'primary.main' }}
                >
                  {contact.firstname.charAt(0)}
                </Avatar>
                <Box sx={{ flex: 1 }}>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="body1" sx={{ fontWeight: 500 }}>
                      {[contact.firstname, contact.nickname && `"${contact.nickname}"`, contact.lastname]
                        .filter(Boolean)
                        .join(' ')}
                    </Typography>
                    {contact.archived && (
                      <Chip
                        label={t('contacts.archived')}
                        size="small"
                        color="default"
                        sx={{ height: 20, fontSize: '0.7rem' }}
                      />
                    )}
                  </Box>
                  <Stack direction="row" spacing={0.5} mt={0.5} flexWrap="wrap" gap={0.5}>
                    {contact.circles &&
                      contact.circles
                        .filter((circle, idx, arr) => arr.indexOf(circle) === idx)
                        .map((circle: string) => (
                          <Chip
                            key={`${contact.ID}-${circle}`}
                            label={circle}
                            size="small"
                            variant="outlined"
                            clickable
                            onClick={(e) => {
                              e.preventDefault();
                              e.stopPropagation();
                              setSelectedCircle(circle);
                              setPage(1);
                            }}
                            sx={{ height: 20, fontSize: '0.75rem' }}
                          />
                        ))}
                  </Stack>
                </Box>
              </Card>
            ))}
          </Stack>
          {!showSelectedOnly && totalContacts > 0 && (
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 2 }}>
              <Pagination
                count={Math.max(1, Math.ceil(totalContacts / pageSize))}
                page={page}
                onChange={(_, value) => {
                  setPage(value);
                }}
                color="primary"
                size="large"
              />
            </Box>
          )}
        </>
      )}
      <AddContactDialog
        open={addDialogOpen}
        onClose={() => {
          setAddDialogOpen(false);
        }}
        onContactAdded={handleContactAdded}
        availableCircles={circles}
        customFieldNames={customFieldNames}
        enabledFields={enabledFields}
      />
      <MergeContactsDialog
        open={mergeOpen}
        onClose={() => {
          setMergeOpen(false);
        }}
        contactIds={selectedIds}
        onMerged={() => {
          setMergeOpen(false);
          setSelected(new Map());
          void refetch();
        }}
      />
      <ImportContactsDialog
        open={importDialogOpen}
        onClose={() => {
          setImportDialogOpen(false);
        }}
        onImportComplete={handleImportComplete}
      />
    </Box>
  );
}
