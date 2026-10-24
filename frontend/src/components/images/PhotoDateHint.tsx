import { useTranslation } from 'react-i18next';
import { Chip } from '@mui/material';
import EventIcon from '@mui/icons-material/Event';

interface PhotoDateHintProps {
  takenAt: string | null | undefined;
  date: string;
  touched: boolean;
  onUse: (date: string) => void;
}

// Offers the capture date of the first image; the date only changes on a click.
export default function PhotoDateHint({ takenAt, date, touched, onUse }: PhotoDateHintProps) {
  const { t, i18n } = useTranslation();
  const taken = takenAt?.slice(0, 10);
  if (!taken || touched || taken === date) return null;

  const label = new Date(`${taken}T12:00:00`).toLocaleDateString(i18n.language, { day: 'numeric', month: 'short' });
  return (
    <Chip
      icon={<EventIcon />}
      label={t('images.usePhotoDate', { date: label })}
      onClick={() => {
        onUse(taken);
      }}
      size="small"
      sx={{ alignSelf: 'flex-start' }}
    />
  );
}
