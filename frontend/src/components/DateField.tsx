import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconButton, InputAdornment, SxProps, TextField, Theme } from '@mui/material';
import CalendarMonthIcon from '@mui/icons-material/CalendarMonth';
import {
  DateFormat,
  autoFormatBirthdayInputWithFormat,
  formatBirthdayForInputWithFormat,
  parseBirthdayInputWithFormat,
  useDateFormat,
} from '../DateFormatProvider';

/**
 * Parses a full date typed in the given format to YYYY-MM-DD.
 * Returns '' for empty input and null for invalid or year-less input.
 */
export function parseDateInputWithFormat(input: string, format: DateFormat): string | null {
  const parsed = parseBirthdayInputWithFormat(input, format);
  if (parsed === null || parsed === '') return parsed;
  if (parsed.startsWith('--')) return null;
  const [year = NaN, month = NaN, day = NaN] = parsed.split('-').map(Number);
  const date = new Date(Date.UTC(year, month - 1, day));
  if (date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return null; // e.g. 31.02.
  return parsed;
}

interface DateFieldProps {
  label: string;
  /** YYYY-MM-DD or '' */
  value: string;
  /** Called with '' while the text is empty or not a complete, valid date, like a native date input */
  onChange: (value: string) => void;
  /** YYYY-MM-DD; limits the calendar picker */
  min?: string;
  /** Message for a value the form refuses, shown like an invalid date */
  error?: string;
  required?: boolean;
  fullWidth?: boolean;
  size?: 'small' | 'medium';
  sx?: SxProps<Theme>;
}

/**
 * Date input in the user's date format. A native date input always shows the
 * browser's locale format, so it is only used behind the calendar button.
 */
export default function DateField({
  label,
  value,
  onChange,
  min,
  error,
  required,
  fullWidth,
  size,
  sx,
}: DateFieldProps) {
  const { t } = useTranslation();
  const { dateFormat, getDatePlaceholder } = useDateFormat();
  const [text, setText] = useState(() => formatBirthdayForInputWithFormat(value, dateFormat));
  const [touched, setTouched] = useState(false);
  const pickerRef = useRef<HTMLInputElement>(null);

  // Follow outside changes (cleared filters, picked dates, a changed date format)
  useEffect(() => {
    if ((parseDateInputWithFormat(text, dateFormat) ?? '') !== value) {
      setText(formatBirthdayForInputWithFormat(value, dateFormat));
      setTouched(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only react to the value and format, not to typing
  }, [value, dateFormat]);

  const handleTextChange = (input: string) => {
    const next = autoFormatBirthdayInputWithFormat(input, text, dateFormat);
    setText(next);
    const parsed = parseDateInputWithFormat(next, dateFormat) ?? '';
    if (parsed !== value) {
      onChange(parsed);
    }
  };

  const openPicker = () => {
    try {
      pickerRef.current?.showPicker();
    } catch {
      pickerRef.current?.focus();
    }
  };

  const invalid = touched && parseDateInputWithFormat(text, dateFormat) === null;

  return (
    <TextField
      size={size}
      label={label}
      required={required}
      fullWidth={fullWidth}
      value={text}
      onChange={(e) => {
        handleTextChange(e.target.value);
      }}
      onBlur={() => {
        setTouched(true);
      }}
      error={invalid || !!error}
      helperText={invalid ? t('common.invalidDate') : error}
      variant="outlined"
      sx={sx}
      slotProps={{
        inputLabel: { shrink: true },
        htmlInput: { placeholder: getDatePlaceholder(), inputMode: 'numeric' },
        input: {
          endAdornment: (
            <InputAdornment position="end" sx={{ position: 'relative' }}>
              <IconButton size="small" edge="end" aria-label={t('common.openCalendar')} onClick={openPicker}>
                <CalendarMonthIcon fontSize="small" />
              </IconButton>
              <input
                ref={pickerRef}
                type="date"
                tabIndex={-1}
                aria-hidden
                value={value}
                min={min}
                onChange={(e) => {
                  onChange(e.target.value);
                }}
                style={{
                  position: 'absolute',
                  right: 0,
                  bottom: 0,
                  width: 0,
                  height: 0,
                  opacity: 0,
                  border: 0,
                  padding: 0,
                }}
              />
            </InputAdornment>
          ),
        },
      }}
    />
  );
}
