import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  FormLabel,
  Radio,
  RadioGroup,
  TextField,
  Typography,
} from '@mui/material';
import AppDialog from './AppDialog';
import { useContactMerge } from '../hooks/useContactMerge';
import { useSnackbar } from '../context/SnackbarContext';
import { useDateFormat } from '../DateFormatProvider';
import {
  CUSTOM_FIELD_PREFIX,
  DECEASED_FIELD,
  DECEASED_UNKNOWN_DATE,
  type MergeContact,
  type MergeCounts,
} from '../utils/mergeContacts';
import { MAX_CONTACT_IMAGES } from '../api/images';

const FIELD_LABELS: Record<string, string> = {
  firstname: 'contacts.firstname',
  lastname: 'contacts.lastname',
  nickname: 'contacts.nickname',
  gender: 'contacts.gender',
  birthday: 'contacts.birthday',
  anniversary: 'contacts.anniversary',
  deceased: 'contacts.deceased',
  prefix: 'contacts.prefix',
  middle_name: 'contacts.middleName',
  suffix: 'contacts.suffix',
  organization: 'contacts.organization',
  department: 'contacts.department',
  job_title: 'contacts.jobTitle',
  role: 'contacts.role',
  how_we_met: 'contacts.howWeMet',
  food_preference: 'contacts.foodPreference',
  work_information: 'contacts.workInformation',
  contact_information: 'contacts.contactInformation',
};

const COUNT_KEYS: (keyof MergeCounts)[] = [
  'emails',
  'phones',
  'addresses',
  'urls',
  'impps',
  'circles',
  'activities',
  'notes',
  'reminders',
  'relationships',
];

export function contactName(c: Pick<MergeContact, 'firstname' | 'lastname'>): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ');
}

interface MergeContactsDialogProps {
  open: boolean;
  onClose: () => void;
  contactIds: number[];
  onMerged: (targetId: number) => void;
}

export default function MergeContactsDialog({ open, onClose, contactIds, onMerged }: MergeContactsDialogProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const displayValue = (field: string, value: string) => {
    if (field !== DECEASED_FIELD) return value;
    return value === DECEASED_UNKNOWN_DATE ? t('contactDetail.deceasedYes') : formatDate(value);
  };
  const { showSuccess } = useSnackbar();
  const { contacts, loading, error, merging, merge, planFor } = useContactMerge(contactIds, open);
  const [targetId, setTargetId] = useState<number | null>(null);
  const [choices, setChoices] = useState<Record<string, string>>({});
  const [customMode, setCustomMode] = useState<Record<string, boolean>>({});
  const [step, setStep] = useState<'review' | 'confirm'>('review');

  const activeTarget = targetId ?? contactIds[0] ?? null;
  const plan = useMemo(() => (activeTarget === null ? null : planFor(activeTarget)), [activeTarget, planFor]);

  useEffect(() => {
    if (open) {
      setTargetId(null);
      setChoices({});
      setCustomMode({});
      setStep('review');
    }
  }, [open]);

  const ordered = contactIds.map((id) => contacts.find((c) => c.ID === id)).filter((c): c is MergeContact => !!c);
  const nameOf = (id: number) => {
    const c = contacts.find((x) => x.ID === id);
    return c ? contactName(c) : '';
  };
  const labelOf = (field: string) =>
    field.startsWith(CUSTOM_FIELD_PREFIX) ? field.slice(CUSTOM_FIELD_PREFIX.length) : t(FIELD_LABELS[field] ?? field);
  const valueOf = (field: string, fallback: string) => choices[field] ?? fallback;

  const handleMerge = async () => {
    if (!plan) return;
    const merged = await merge(plan.target.ID, { ...plan.defaults, ...choices });
    if (merged) {
      showSuccess(t('merge.done', { name: contactName(plan.target) }));
      onMerged(plan.target.ID);
    }
  };

  const items = plan
    ? COUNT_KEYS.filter((k) => plan.counts[k] > 0).map((k) => t(`merge.count.${k}`, { count: plan.counts[k] }))
    : [];

  return (
    <AppDialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{step === 'review' ? t('merge.title') : t('merge.confirmTitle')}</DialogTitle>
      <DialogContent>
        {loading && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <CircularProgress size={20} />
            <Typography>{t('merge.loading')}</Typography>
          </Box>
        )}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        {plan && step === 'review' && (
          <>
            {plan.tooManyImages && (
              <Alert severity="error" sx={{ mb: 2 }}>
                {t('merge.tooManyImages', { count: plan.imageCount, max: MAX_CONTACT_IMAGES })}
              </Alert>
            )}
            <FormControl sx={{ mb: 2 }}>
              <FormLabel id="merge-target-label">{t('merge.targetLegend')}</FormLabel>
              <Typography variant="caption" color="text.secondary">
                {t('merge.targetHint')}
              </Typography>
              <RadioGroup
                aria-labelledby="merge-target-label"
                value={String(plan.target.ID)}
                onChange={(e) => {
                  setTargetId(Number(e.target.value));
                  setChoices({});
                  setCustomMode({});
                }}
              >
                {ordered.map((c) => (
                  <FormControlLabel
                    key={c.ID}
                    value={String(c.ID)}
                    control={<Radio />}
                    label={[contactName(c), c.emails?.[0]?.value].filter(Boolean).join(' · ')}
                  />
                ))}
              </RadioGroup>
            </FormControl>

            {plan.conflicts.length === 0 ? (
              <Typography variant="body2" color="text.secondary">
                {t('merge.noConflicts')}
              </Typography>
            ) : (
              <>
                <Typography variant="subtitle1">{t('merge.conflictsTitle')}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  {t('merge.conflictsHint')}
                </Typography>
              </>
            )}
            {plan.conflicts.map((conflict) => {
              const fallback = plan.defaults[conflict.field] ?? '';
              const current = valueOf(conflict.field, fallback);
              const custom = customMode[conflict.field] ?? false;
              return (
                <FormControl key={conflict.field} sx={{ display: 'block', mb: 2 }}>
                  <FormLabel id={`merge-field-${conflict.field}`}>{labelOf(conflict.field)}</FormLabel>
                  <RadioGroup
                    aria-labelledby={`merge-field-${conflict.field}`}
                    value={custom ? '__custom__' : current}
                    onChange={(e) => {
                      if (e.target.value === '__custom__') {
                        setCustomMode((m) => ({ ...m, [conflict.field]: true }));
                        setChoices((c) => ({ ...c, [conflict.field]: valueOf(conflict.field, fallback) }));
                      } else {
                        setCustomMode((m) => ({ ...m, [conflict.field]: false }));
                        setChoices((c) => ({ ...c, [conflict.field]: e.target.value }));
                      }
                    }}
                  >
                    {[...new Set(conflict.values.map((v) => v.value))].map((value) => (
                      <FormControlLabel
                        key={value}
                        value={value}
                        control={<Radio />}
                        label={
                          <>
                            {displayValue(conflict.field, value)}
                            <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                              {conflict.values
                                .filter((v) => v.value === value)
                                .map((v) => nameOf(v.contactId))
                                .join(', ')}
                            </Typography>
                          </>
                        }
                      />
                    ))}
                    {conflict.field !== DECEASED_FIELD && (
                      <FormControlLabel value="__custom__" control={<Radio />} label={t('merge.otherValue')} />
                    )}
                  </RadioGroup>
                  {custom && (
                    <TextField
                      size="small"
                      fullWidth
                      value={current}
                      label={t('merge.customValue', { field: labelOf(conflict.field) })}
                      onChange={(e) => {
                        setChoices((c) => ({ ...c, [conflict.field]: e.target.value }));
                      }}
                    />
                  )}
                </FormControl>
              );
            })}
          </>
        )}
        {plan && step === 'confirm' && (
          <>
            <Typography sx={{ mb: 1 }}>
              {items.length > 0
                ? t('merge.moves', { items: items.join(', '), name: contactName(plan.target) })
                : t('merge.nothingMoves', { name: contactName(plan.target) })}{' '}
              {t('merge.deletes', { count: plan.sources.length })}
            </Typography>
            <Alert severity="warning">{t('merge.cannotUndo')}</Alert>
          </>
        )}
      </DialogContent>
      <DialogActions>
        {step === 'review' ? (
          <>
            <Button onClick={onClose}>{t('common.cancel')}</Button>
            <Button
              variant="contained"
              disabled={!plan || plan.tooManyImages}
              onClick={() => {
                setStep('confirm');
              }}
            >
              {t('common.continue')}
            </Button>
          </>
        ) : (
          <>
            <Button
              disabled={merging}
              onClick={() => {
                setStep('review');
              }}
            >
              {t('common.back')}
            </Button>
            <Button variant="contained" color="error" disabled={merging} onClick={() => void handleMerge()}>
              {merging ? t('merge.merging') : t('merge.confirm')}
            </Button>
          </>
        )}
      </DialogActions>
    </AppDialog>
  );
}
