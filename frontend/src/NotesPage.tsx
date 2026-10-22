import { useSearchParams } from 'react-router-dom';
import { useState, useMemo, useEffect, ChangeEvent, MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Typography, Paper, TextField, Button, IconButton, Pagination, Popover, Stack } from '@mui/material';
import {
  Timeline,
  TimelineItem,
  TimelineSeparator,
  TimelineConnector,
  TimelineContent,
  TimelineDot,
  TimelineOppositeContent,
} from '@mui/lab';
import NoteIcon from '@mui/icons-material/Note';
import EditIcon from '@mui/icons-material/Edit';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { ListSkeleton } from './components/LoadingSkeletons';
import DateField from './components/DateField';
import { useNotes } from './hooks/useNotes';
import { useDebouncedValue } from './hooks/useDebounce';
import { createUnassignedNote, updateNote, deleteNote, Note } from './api/notes';
import AddNoteDialog from './components/AddNoteDialog';
import EditTimelineItemDialog, { type TimelineItemValues } from './components/EditTimelineItemDialog';
import ImagePreview from './components/images/ImagePreview';
import ImageLightbox from './components/images/ImageLightbox';
import { handleError } from './utils/errorHandler';
import { useDateFormat } from './DateFormatProvider';
import { now } from './utils/now';

const NotesPage: React.FC = () => {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [searchParams] = useSearchParams();
  const urlSearch = searchParams.get('search') ?? '';
  const [searchInput, setSearchInput] = useState(urlSearch);
  const debouncedSearch = useDebouncedValue(searchInput, 400);
  const [page, setPage] = useState(1);

  // A link to this page with a term (from the search) replaces the filter
  useEffect(() => {
    setSearchInput(urlSearch);
    setPage(1);
  }, [urlSearch]);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const NOTES_PER_PAGE = 25;

  const notesParams = useMemo(
    () => ({
      page,
      limit: NOTES_PER_PAGE,
      search: debouncedSearch.trim() || undefined,
      fromDate: fromDate || undefined,
      toDate: toDate || undefined,
    }),
    [page, debouncedSearch, fromDate, toDate],
  );

  const { notes, total, page: serverPage, limit, loading, refetch } = useNotes(undefined, notesParams);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [editingNote, setEditingNote] = useState<Note | null>(null);
  const [editValues, setEditValues] = useState<TimelineItemValues>({});
  const [lightbox, setLightbox] = useState<{ note: Note; index: number } | null>(null);
  const [infoAnchorEl, setInfoAnchorEl] = useState<HTMLElement | null>(null);
  const infoOpen = Boolean(infoAnchorEl);
  const infoPopoverId = infoOpen ? 'notes-info-popover' : undefined;

  const handleSearchChange = (value: string) => {
    setSearchInput(value);
    setPage(1);
  };

  const handleFromDateChange = (value: string) => {
    setFromDate(value);
    setPage(1);
  };

  const handleToDateChange = (value: string) => {
    setToDate(value);
    setPage(1);
  };

  const handlePageChange = (_: ChangeEvent<unknown>, value: number) => {
    setPage(value);
  };

  const currentPage = serverPage || page;
  const pageSize = limit || NOTES_PER_PAGE;
  const totalPages = Math.max(1, Math.ceil((total || 0) / pageSize));

  const handleAddNote = () => {
    setAddDialogOpen(true);
  };

  const handleNoteSave = async (content: string, date: string, imageIds: number[]) => {
    try {
      await createUnassignedNote({ content, date: new Date(date).toISOString(), image_ids: imageIds });
      setAddDialogOpen(false);
      refetch();
    } catch (err) {
      handleError(err, { operation: 'creating note' });
      throw err;
    }
  };

  const handleEditClick = (note: Note) => {
    setEditingNote(note);
    setEditValues({
      noteContent: note.content || '',
      noteImages: note.images,
      noteDate: note.date ? new Date(note.date).toISOString().split('T')[0] : '',
    });
  };

  const handleSaveEdit = async (imageIds: number[]) => {
    if (!editingNote || (!editValues.noteContent?.trim() && imageIds.length === 0)) return;

    try {
      await updateNote(editingNote.ID, {
        content: editValues.noteContent ?? '',
        date: editValues.noteDate ? new Date(editValues.noteDate).toISOString() : now().toISOString(),
        image_ids: imageIds,
      });
      setEditingNote(null);
      setEditValues({});
      refetch();
    } catch (err) {
      handleError(err, { operation: 'updating note' });
    }
  };

  const handleCancelEdit = () => {
    setEditingNote(null);
    setEditValues({});
  };

  const handleDeleteNote = async () => {
    if (!editingNote) return;

    try {
      await deleteNote(editingNote.ID);
      setEditingNote(null);
      setEditValues({});
      refetch();
    } catch (err) {
      handleError(err, { operation: 'deleting note' });
    }
  };

  const handleInfoClick = (event: MouseEvent<HTMLElement>) => {
    setInfoAnchorEl(event.currentTarget);
  };

  const handleInfoClose = () => {
    setInfoAnchorEl(null);
  };

  const isInitialLoading = loading && notes.length === 0;
  const hasFilters = searchInput.trim().length > 0 || fromDate || toDate;

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Box display="flex" alignItems="center" gap={1} mb={2}>
        <Typography variant="h5">{t('notes.title')}</Typography>
        <IconButton size="small" aria-describedby={infoPopoverId} onClick={handleInfoClick}>
          <InfoOutlinedIcon fontSize="small" />
        </IconButton>
      </Box>
      <Popover
        id={infoPopoverId}
        open={infoOpen}
        anchorEl={infoAnchorEl}
        onClose={handleInfoClose}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
      >
        <Box sx={{ p: 2, maxWidth: 320 }}>
          <Typography variant="body2">{t('notes.unassignedInfo')}</Typography>
        </Box>
      </Popover>

      <TextField
        size="small"
        fullWidth
        label={t('notes.search')}
        value={searchInput}
        onChange={(e) => {
          handleSearchChange(e.target.value);
        }}
        sx={{ mb: 1.5 }}
      />
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems={{ xs: 'stretch', sm: 'center' }}>
        <DateField
          size="small"
          label={t('notes.fromDate')}
          value={fromDate}
          onChange={handleFromDateChange}
          sx={{ width: { sm: 180 } }}
        />
        <DateField
          size="small"
          label={t('notes.toDate')}
          value={toDate}
          onChange={handleToDateChange}
          sx={{ width: { sm: 180 } }}
        />
        <Button
          variant="contained"
          startIcon={<NoteIcon />}
          onClick={handleAddNote}
          sx={{ ml: { sm: 'auto' }, whiteSpace: 'nowrap' }}
        >
          {t('notes.addNote')}
        </Button>
      </Stack>

      {isInitialLoading ? (
        <ListSkeleton count={8} />
      ) : notes.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            {hasFilters ? t('notes.noResults') : t('notes.noNotes')}
          </Typography>
        </Paper>
      ) : (
        <Timeline position="right">
          {notes.map((note, index) => (
            <TimelineItem key={note.ID}>
              <TimelineOppositeContent color="text.secondary" sx={{ flex: 0.2 }}>
                <Typography variant="body2">{formatDate(note.date)}</Typography>
              </TimelineOppositeContent>
              <TimelineSeparator>
                <TimelineDot color="primary">
                  <NoteIcon />
                </TimelineDot>
                {index < notes.length - 1 && <TimelineConnector />}
              </TimelineSeparator>
              <TimelineContent sx={{ flex: 0.8 }}>
                <Paper
                  elevation={2}
                  sx={{
                    p: 2,
                    '&:hover .edit-actions': {
                      opacity: 1,
                    },
                  }}
                >
                  <Box>
                    <Box display="flex" justifyContent="space-between" alignItems="flex-start">
                      <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', flex: 1 }}>
                        {note.content}
                      </Typography>
                      <Box
                        className="edit-actions"
                        sx={{ opacity: 0, transition: 'opacity 0.2s', display: 'flex', gap: 1 }}
                      >
                        <IconButton
                          size="small"
                          onClick={() => {
                            handleEditClick(note);
                          }}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    </Box>
                    <ImagePreview
                      images={note.images}
                      onOpen={(index) => {
                        setLightbox({ note, index });
                      }}
                    />
                  </Box>
                </Paper>
              </TimelineContent>
            </TimelineItem>
          ))}
        </Timeline>
      )}

      {totalPages > 1 && (
        <Box display="flex" justifyContent="center" mt={3}>
          <Pagination
            color="primary"
            count={totalPages}
            page={currentPage}
            onChange={handlePageChange}
            showFirstButton
            showLastButton
          />
        </Box>
      )}

      <AddNoteDialog
        open={addDialogOpen}
        onClose={() => {
          setAddDialogOpen(false);
        }}
        onSave={handleNoteSave}
      />

      <ImageLightbox
        images={lightbox?.note.images ?? []}
        index={lightbox?.index ?? null}
        onClose={() => {
          setLightbox(null);
        }}
      />

      {editingNote && (
        <EditTimelineItemDialog
          open={!!editingNote}
          onClose={handleCancelEdit}
          onSave={handleSaveEdit}
          onDelete={handleDeleteNote}
          type="note"
          values={editValues}
          onChange={setEditValues}
        />
      )}
    </Box>
  );
};

export default NotesPage;
