import { useState, useEffect } from 'react';
import { TextField, Autocomplete, CircularProgress } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { getActivities, Activity } from '../api/activities';
import { useDateFormat } from '../DateFormatProvider';
import { useDebouncedValue } from '../hooks/useDebounce';

interface ExistingActivitiesPickerProps {
  contactId: number;
  value: Activity[];
  onChange: (activities: Activity[]) => void;
}

export default function ExistingActivitiesPicker({ contactId, value, onChange }: ExistingActivitiesPickerProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [options, setOptions] = useState<Activity[]>([]);
  const [searchInput, setSearchInput] = useState('');
  const [loading, setLoading] = useState(false);

  const debouncedSearch = useDebouncedValue(searchInput, 300);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getActivities({ limit: 50, includeContacts: true, search: debouncedSearch })
      .then((response) => {
        if (!cancelled) setOptions(response.activities);
      })
      .catch((err: unknown) => {
        console.error('Failed to fetch activities:', err);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [debouncedSearch]);

  const label = (activity: Activity) => `${activity.title} (${formatDate(activity.date)})`;
  const available = options.filter((a) => !(a.contacts ?? []).some((c) => c.ID === contactId));

  return (
    <Autocomplete
      multiple
      options={available}
      getOptionLabel={label}
      value={value}
      onChange={(_, newValue) => {
        onChange(newValue);
      }}
      onInputChange={(_, input, reason) => {
        if (reason !== 'reset') setSearchInput(input);
      }}
      loading={loading}
      filterOptions={(x) => x}
      isOptionEqualToValue={(option, selected) => option.ID === selected.ID}
      noOptionsText={t('activityDialog.noActivities')}
      renderInput={(params) => (
        <TextField
          {...params}
          label={t('activityDialog.activities')}
          placeholder={t('activityDialog.selectActivities')}
          slotProps={{
            input: {
              ...params.InputProps,
              endAdornment: (
                <>
                  {loading ? <CircularProgress color="inherit" size={20} /> : null}
                  {params.InputProps.endAdornment}
                </>
              ),
            },
          }}
        />
      )}
    />
  );
}
