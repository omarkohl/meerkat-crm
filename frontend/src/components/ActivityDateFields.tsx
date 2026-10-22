import { useTranslation } from 'react-i18next';
import { Box } from '@mui/material';
import DateField from './DateField';
import { endsBeforeStart } from '../utils/activityDates';

interface ActivityDateFieldsProps {
  /** YYYY-MM-DD or '' */
  date: string;
  /** YYYY-MM-DD or '' */
  endDate: string;
  onChange: (dates: { date: string; endDate: string }) => void;
  required?: boolean;
}

export default function ActivityDateFields({ date, endDate, onChange, required }: ActivityDateFieldsProps) {
  const { t } = useTranslation();

  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 2 }}>
      <DateField
        label={t('activityDialog.date')}
        value={date}
        onChange={(next) => {
          onChange({ date: next, endDate: endsBeforeStart(next, endDate) ? next : endDate });
        }}
        fullWidth
        required={required}
      />
      <DateField
        label={t('activityDialog.endDate')}
        value={endDate}
        onChange={(next) => {
          onChange({ date, endDate: next });
        }}
        min={date || undefined}
        error={endsBeforeStart(date, endDate) ? t('activityDialog.endBeforeStart') : undefined}
        fullWidth
      />
    </Box>
  );
}
