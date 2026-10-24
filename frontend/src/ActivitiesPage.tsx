import { useState, useMemo, useEffect, ChangeEvent, MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { Box, Typography, Paper, TextField, Button, IconButton, Chip, Pagination, Popover, Stack } from '@mui/material';
import {
  Timeline,
  TimelineItem,
  TimelineSeparator,
  TimelineConnector,
  TimelineContent,
  TimelineDot,
  TimelineOppositeContent,
} from '@mui/lab';
import EventIcon from '@mui/icons-material/Event';
import EditIcon from '@mui/icons-material/Edit';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { useActivities } from './hooks/useActivities';
import { useDebouncedValue } from './hooks/useDebounce';
import { createActivity, updateActivity, deleteActivity, Activity } from './api/activities';
import { Contact } from './api/contacts';
import AddActivityDialog from './components/AddActivityDialog';
import EditTimelineItemDialog, { type TimelineItemValues } from './components/EditTimelineItemDialog';
import DateField from './components/DateField';
import { ListSkeleton } from './components/LoadingSkeletons';
import { useDateFormat } from './DateFormatProvider';
import { now } from './utils/now';
import { dayOf, endDateToSend } from './utils/activityDates';
import ImagePreview from './components/images/ImagePreview';
import ImageLightbox from './components/images/ImageLightbox';

const ActivitiesPage: React.FC = () => {
  const { t } = useTranslation();
  const { formatDateRange } = useDateFormat();

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
  const [lightbox, setLightbox] = useState<{ activity: Activity; index: number } | null>(null);
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const ACTIVITIES_PER_PAGE = 25;

  // Memoize params to prevent unnecessary refetching
  const activityParams = useMemo(
    () => ({
      includeContacts: true,
      page,
      limit: ACTIVITIES_PER_PAGE,
      search: debouncedSearch.trim() || undefined,
      fromDate: fromDate || undefined,
      toDate: toDate || undefined,
    }),
    [page, debouncedSearch, fromDate, toDate],
  );

  const { activities, total, page: serverPage, limit, loading, refetch } = useActivities(activityParams);
  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const [editingActivity, setEditingActivity] = useState<Activity | null>(null);
  const [editValues, setEditValues] = useState<TimelineItemValues>({});
  const [infoAnchorEl, setInfoAnchorEl] = useState<HTMLElement | null>(null);
  const infoOpen = Boolean(infoAnchorEl);
  const infoPopoverId = infoOpen ? 'activities-info-popover' : undefined;

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
  const pageSize = limit || ACTIVITIES_PER_PAGE;
  const totalPages = Math.max(1, Math.ceil((total || 0) / pageSize));

  const handleAddActivity = () => {
    setAddDialogOpen(true);
  };

  const handleActivitySave = async (activity: {
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
        ...activity,
        date: new Date(activity.date).toISOString(),
        end_date: endDateToSend(activity.date, activity.end_date),
      });
      setAddDialogOpen(false);
      refetch();
    } catch (err) {
      console.error('Failed to create activity:', err);
      throw err;
    }
  };

  const handleEditClick = (activity: Activity) => {
    setEditingActivity(activity);

    setEditValues({
      activityTitle: activity.title || '',
      activityDescription: activity.description || '',
      activityLocation: activity.location || '',
      activityDate: activity.date ? new Date(activity.date).toISOString().split('T')[0] : '',
      activityEndDate: dayOf(activity.end_date),
      activityImages: activity.images,
      activityContacts: activity.contacts || [],
    });
  };

  const handleSaveEdit = async (imageIds: number[]) => {
    if (!editingActivity || !editValues.activityTitle?.trim()) return;

    try {
      await updateActivity(editingActivity.ID, {
        title: editValues.activityTitle,
        description: editValues.activityDescription || '',
        location: editValues.activityLocation || '',
        date: editValues.activityDate ? new Date(editValues.activityDate).toISOString() : now().toISOString(),
        end_date: endDateToSend(editValues.activityDate ?? '', editValues.activityEndDate),
        contact_ids: editValues.activityContacts?.map((c) => c.ID) || [],
        image_ids: imageIds,
      });
      setEditingActivity(null);
      setEditValues({});
      refetch();
    } catch (err) {
      console.error('Failed to update activity:', err);
    }
  };

  const handleCancelEdit = () => {
    setEditingActivity(null);
    setEditValues({});
  };

  const handleDeleteActivity = async () => {
    if (!editingActivity) return;

    try {
      await deleteActivity(editingActivity.ID);
      setEditingActivity(null);
      setEditValues({});
      refetch();
    } catch (err) {
      console.error('Failed to delete activity:', err);
    }
  };

  const handleInfoClick = (event: MouseEvent<HTMLElement>) => {
    setInfoAnchorEl(event.currentTarget);
  };

  const handleInfoClose = () => {
    setInfoAnchorEl(null);
  };

  const isInitialLoading = loading && activities.length === 0;
  const hasFilters = searchInput.trim().length > 0 || fromDate || toDate;

  return (
    <Box sx={{ maxWidth: 1200, mx: 'auto', mt: 2, p: 2 }}>
      <Box display="flex" alignItems="center" gap={1} mb={2}>
        <Typography variant="h5">{t('activities.title')}</Typography>
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
          <Typography variant="body2">{t('activities.allContactsInfo')}</Typography>
        </Box>
      </Popover>

      <TextField
        size="small"
        fullWidth
        label={t('activities.search')}
        value={searchInput}
        onChange={(e) => {
          handleSearchChange(e.target.value);
        }}
        sx={{ mb: 1.5 }}
      />
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} mb={2} alignItems={{ xs: 'stretch', sm: 'center' }}>
        <DateField
          size="small"
          label={t('activities.fromDate')}
          value={fromDate}
          onChange={handleFromDateChange}
          sx={{ width: { sm: 180 } }}
        />
        <DateField
          size="small"
          label={t('activities.toDate')}
          value={toDate}
          onChange={handleToDateChange}
          sx={{ width: { sm: 180 } }}
        />
        <Button
          variant="contained"
          startIcon={<EventIcon />}
          onClick={handleAddActivity}
          sx={{ ml: { sm: 'auto' }, whiteSpace: 'nowrap' }}
        >
          {t('activities.addActivity')}
        </Button>
      </Stack>

      {isInitialLoading ? (
        <ListSkeleton count={8} />
      ) : activities.length === 0 ? (
        <Paper sx={{ p: 4, textAlign: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            {hasFilters ? t('activities.noResults') : t('activities.noActivities')}
          </Typography>
        </Paper>
      ) : (
        <Timeline position="right">
          {activities.map((activity, index) => (
            <TimelineItem key={activity.ID}>
              <TimelineOppositeContent color="text.secondary" sx={{ flex: 0.2 }}>
                <Typography variant="body2">{formatDateRange(activity.date, activity.end_date)}</Typography>
              </TimelineOppositeContent>
              <TimelineSeparator>
                <TimelineDot color="primary">
                  <EventIcon />
                </TimelineDot>
                {index < activities.length - 1 && <TimelineConnector />}
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
                      <Box sx={{ flex: 1, pr: 2 }}>
                        {activity.title && (
                          <Typography
                            variant="body2"
                            fontWeight={600}
                            sx={{ wordBreak: 'break-word', color: 'text.primary' }}
                          >
                            {activity.title}
                          </Typography>
                        )}
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', mt: activity.title ? 0.5 : 0 }}>
                          {activity.description}
                        </Typography>
                        {activity.location && (
                          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                            📍 {activity.location}
                          </Typography>
                        )}
                      </Box>
                      <Box
                        className="edit-actions"
                        sx={{ opacity: 0, transition: 'opacity 0.2s', display: 'flex', gap: 1 }}
                      >
                        <IconButton
                          size="small"
                          onClick={() => {
                            handleEditClick(activity);
                          }}
                        >
                          <EditIcon fontSize="small" />
                        </IconButton>
                      </Box>
                    </Box>
                    <ImagePreview
                      images={activity.images}
                      onOpen={(index) => {
                        setLightbox({ activity, index });
                      }}
                    />
                    {activity.contacts && activity.contacts.length > 0 && (
                      <Box mt={1} display="flex" flexWrap="wrap" gap={0.5}>
                        {activity.contacts.map((contact: Contact) => (
                          <Chip
                            key={contact.ID}
                            label={`${contact.firstname} ${contact.lastname}`}
                            size="small"
                            variant="outlined"
                            component={RouterLink}
                            to={`/contacts/${contact.ID}`}
                            clickable
                          />
                        ))}
                      </Box>
                    )}
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

      <AddActivityDialog
        open={addDialogOpen}
        onClose={() => {
          setAddDialogOpen(false);
        }}
        onSave={handleActivitySave}
      />

      <ImageLightbox
        images={lightbox?.activity.images ?? []}
        index={lightbox?.index ?? null}
        onClose={() => {
          setLightbox(null);
        }}
      />

      {editingActivity && (
        <EditTimelineItemDialog
          open={!!editingActivity}
          onClose={handleCancelEdit}
          onSave={handleSaveEdit}
          onDelete={handleDeleteActivity}
          type="activity"
          values={editValues}
          onChange={setEditValues}
        />
      )}
    </Box>
  );
};

export default ActivitiesPage;
