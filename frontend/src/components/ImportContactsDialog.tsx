import { useState, useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Stepper,
  Step,
  StepLabel,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Select,
  MenuItem,
  FormControl,
  FormHelperText,
  Chip,
  Alert,
  LinearProgress,
  IconButton,
  Tooltip,
  TableSortLabel,
} from '@mui/material';
import AppDialog from './AppDialog';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import WarningIcon from '@mui/icons-material/Warning';
import ErrorIcon from '@mui/icons-material/Error';
import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import RestoreFromTrashIcon from '@mui/icons-material/RestoreFromTrash';
import CloseIcon from '@mui/icons-material/Close';
import { useSnackbar } from '../context/SnackbarContext';
import { getErrorMessage } from '../utils/errorHandler';
import {
  uploadCSVForImport,
  uploadVCFForImport,
  getImportPreview,
  confirmImport,
  confirmVCFImport,
  getVCFImportStatus,
  VCFPhotoStatus,
  ColumnMapping,
  ImportUploadResponse,
  ImportPreviewResponse,
  RowImportAction,
  ImportResult,
  ImportRowPreview,
  IMPORTABLE_CONTACT_FIELDS,
  CONTACT_FIELD_LABELS,
  REPEATABLE_VALUE_FIELDS,
} from '../api/import';
import { PreviewFilter, PreviewSort, PreviewSortKey, filterRowIndices, sortRows } from './importPreviewRows';
import EmptyValue from './EmptyValue';

interface ImportContactsDialogProps {
  open: boolean;
  onClose: () => void;
  onImportComplete: () => void;
}

type ImportStep = 'upload' | 'mapping' | 'preview' | 'result';
type ImportType = 'csv' | 'vcf';

const CSV_STEP_KEYS = ['upload', 'mapColumns', 'review', 'done'] as const;
const VCF_STEP_KEYS = ['upload', 'review', 'done'] as const;
const PHOTO_POLL_INTERVAL_MS = 1000;

export default function ImportContactsDialog({ open, onClose, onImportComplete }: ImportContactsDialogProps) {
  const { t } = useTranslation();
  const { showSuccess } = useSnackbar();

  // Step state
  const [activeStep, setActiveStep] = useState(0);
  const [step, setStep] = useState<ImportStep>('upload');
  const [importType, setImportType] = useState<ImportType>('csv');

  // Upload state
  const [uploadResponse, setUploadResponse] = useState<ImportUploadResponse | null>(null);
  const [mappings, setMappings] = useState<ColumnMapping[]>([]);

  // Preview state
  const [previewResponse, setPreviewResponse] = useState<ImportPreviewResponse | null>(null);
  const [rowActions, setRowActions] = useState<Map<number, string>>(new Map());
  const [filter, setFilter] = useState<{ key: PreviewFilter; rows: Set<number> } | null>(null);
  const [sort, setSort] = useState<PreviewSort>({ key: 'row', direction: 'asc' });

  // Result state
  const [importResult, setImportResult] = useState<ImportResult | null>(null);
  const [photoStatus, setPhotoStatus] = useState<VCFPhotoStatus | null>(null);
  const [photoPollFailed, setPhotoPollFailed] = useState(false);

  // UI state
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);

  // Reset dialog state
  const resetDialog = useCallback(() => {
    setActiveStep(0);
    setStep('upload');
    setImportType('csv');
    setUploadResponse(null);
    setMappings([]);
    setPreviewResponse(null);
    setRowActions(new Map());
    setFilter(null);
    setSort({ key: 'row', direction: 'asc' });
    setImportResult(null);
    setPhotoStatus(null);
    setPhotoPollFailed(false);
    setLoading(false);
    setError(null);
    setDragOver(false);
  }, []);

  const onImportCompleteRef = useRef(onImportComplete);
  onImportCompleteRef.current = onImportComplete;
  const photoSessionId =
    importType === 'vcf' && importResult && importResult.photos_queued > 0 ? previewResponse?.session_id : undefined;

  useEffect(() => {
    if (!open || !photoSessionId) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      try {
        const status = await getVCFImportStatus(photoSessionId);
        if (stopped) return;
        setPhotoStatus(status);
        if (status.phase !== 'done') timer = setTimeout(() => void poll(), PHOTO_POLL_INTERVAL_MS);
        else if (status.photos_saved > 0) onImportCompleteRef.current();
      } catch {
        if (!stopped) setPhotoPollFailed(true);
      }
    };
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [open, photoSessionId]);

  // Handle dialog close
  const handleClose = () => {
    resetDialog();
    onClose();
  };

  // Handle file upload
  const handleFileUpload = async (file: File) => {
    const fileName = file.name.toLowerCase();
    const isCSV = fileName.endsWith('.csv');
    const isVCF = fileName.endsWith('.vcf');

    if (!isCSV && !isVCF) {
      setError(t('contacts.import.errors.invalidFile', 'Please select a valid CSV or VCF file'));
      return;
    }

    const maxSize = isVCF ? 10 * 1024 * 1024 : 5 * 1024 * 1024; // 10MB for VCF, 5MB for CSV
    if (file.size > maxSize) {
      setError(
        t('contacts.import.errors.fileTooLarge', 'File is too large. Maximum size is {{size}}MB', {
          size: maxSize / (1024 * 1024),
        }),
      );
      return;
    }

    setLoading(true);
    setError(null);

    try {
      if (isVCF) {
        // VCF import - goes directly to preview (no mapping needed)
        setImportType('vcf');
        const response = await uploadVCFForImport(file);
        setPreviewResponse(response);

        // Initialize row actions based on suggested actions
        const initialActions = new Map<number, string>();
        response.rows.forEach((row) => {
          initialActions.set(row.row_index, row.suggested_action);
        });
        setRowActions(initialActions);

        setStep('preview');
        setActiveStep(1); // VCF skips mapping, so preview is step 1
      } else {
        // CSV import - needs column mapping
        setImportType('csv');
        const response = await uploadCSVForImport(file);
        setUploadResponse(response);
        setMappings(response.suggested_mappings);
        setStep('mapping');
        setActiveStep(1);
      }
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Handle file input change
  const handleFileInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      handleFileUpload(file);
    }
    // Reset input value to allow re-selecting the same file
    event.target.value = '';
  };

  // Handle drag and drop
  const handleDragOver = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(true);
  };

  const handleDragLeave = () => {
    setDragOver(false);
  };

  const handleDrop = (event: React.DragEvent) => {
    event.preventDefault();
    setDragOver(false);
    const file = event.dataTransfer.files?.[0];
    if (file) {
      handleFileUpload(file);
    }
  };

  // Handle mapping change
  const handleMappingChange = (index: number, field: string) => {
    const newMappings = [...mappings];
    const current = newMappings[index];
    if (!current) return;
    newMappings[index] = { ...current, contact_field: field };
    setMappings(newMappings);
  };

  // Handle preview generation
  const handleGeneratePreview = async () => {
    if (!uploadResponse) return;

    // Check if at least one field is mapped
    const hasMappings = mappings.some((m) => m.contact_field !== '');
    if (!hasMappings) {
      setError(t('contacts.import.errors.noMappings', 'Please map at least one column'));
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await getImportPreview(uploadResponse.session_id, mappings);
      setPreviewResponse(response);

      // Initialize row actions based on suggested actions
      const initialActions = new Map<number, string>();
      response.rows.forEach((row) => {
        initialActions.set(row.row_index, row.suggested_action);
      });
      setRowActions(initialActions);

      setStep('preview');
      setActiveStep(2);
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Handle row action change
  const handleRowActionChange = (rowIndex: number, action: string) => {
    const newActions = new Map(rowActions);
    newActions.set(rowIndex, action);
    setRowActions(newActions);
  };

  // Handle import confirmation
  const handleConfirmImport = async () => {
    if (!previewResponse) return;

    setLoading(true);
    setError(null);

    try {
      const actions: RowImportAction[] = [];
      rowActions.forEach((action, rowIndex) => {
        actions.push({ row_index: rowIndex, action: action as 'skip' | 'add' | 'update' });
      });

      // Use appropriate confirm endpoint based on import type
      const result =
        importType === 'vcf'
          ? await confirmVCFImport(previewResponse.session_id, actions)
          : await confirmImport(previewResponse.session_id, actions);

      setImportResult(result);
      setStep('result');
      setActiveStep(importType === 'vcf' ? 2 : 3); // VCF has fewer steps

      if (result.created > 0 || result.updated > 0) {
        showSuccess(
          t('contacts.import.result.success', 'Import completed: {{created}} created, {{updated}} updated', {
            created: result.created,
            updated: result.updated,
          }),
        );
        onImportComplete();
      }
    } catch (err) {
      setError(getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  // Calculate summary counts
  const getSummaryCounts = () => {
    let toCreate = 0;
    let toUpdate = 0;
    let toSkip = 0;

    rowActions.forEach((action) => {
      if (action === 'add') toCreate++;
      else if (action === 'update') toUpdate++;
      else toSkip++;
    });

    return { toCreate, toUpdate, toSkip };
  };

  // Clicking a summary chip shows only its rows; clicking it again shows all.
  const toggleFilter = (key: PreviewFilter) => {
    if (!previewResponse || filter?.key === key) {
      setFilter(null);
      return;
    }
    setFilter({ key, rows: filterRowIndices(previewResponse.rows, rowActions, key) });
  };

  const toggleSort = (key: PreviewSortKey) => {
    setSort((prev) => ({
      key,
      direction: prev.key === key && prev.direction === 'asc' ? 'desc' : 'asc',
    }));
  };

  // Sets one action on every shown row that allows it.
  const applyActionToRows = (rows: ImportRowPreview[], action: string) => {
    const newActions = new Map(rowActions);
    rows.forEach((row) => {
      if (row.validation_errors.length > 0) return;
      if (action === 'update' && !row.duplicate_match) return;
      newActions.set(row.row_index, action);
    });
    setRowActions(newActions);
  };

  // Render upload step
  const renderUploadStep = () => (
    <Box sx={{ py: 4 }}>
      <Box
        sx={{
          border: '2px dashed',
          borderColor: dragOver ? 'primary.main' : 'grey.400',
          borderRadius: 2,
          p: 6,
          textAlign: 'center',
          cursor: 'pointer',
          bgcolor: dragOver ? 'action.hover' : 'background.paper',
          transition: 'all 0.2s',
        }}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => document.getElementById('import-file-input')?.click()}
      >
        <input
          id="import-file-input"
          type="file"
          accept=".csv,.vcf"
          style={{ display: 'none' }}
          onChange={handleFileInputChange}
        />
        <CloudUploadIcon sx={{ fontSize: 48, color: 'grey.500', mb: 2 }} />
        <Typography variant="h6" gutterBottom>
          {t('contacts.import.upload.dragDrop', 'Drag and drop a CSV or VCF file here, or click to select')}
        </Typography>
        <Typography variant="body2" color="text.secondary">
          {t('contacts.import.upload.supportedFormats', 'Supported formats: CSV (spreadsheet), VCF (vCard)')}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {t('contacts.import.upload.maxSize', 'Maximum file size: 10MB')}
        </Typography>
      </Box>
    </Box>
  );

  // Render mapping step
  const renderMappingStep = () => {
    if (!uploadResponse) return null;

    // "field#group" keys of single-slot targets mapped by more than one column. Repeatable
    // multi-value fields (email/phone/website/IM) are excluded — multiple columns there
    // legitimately become separate entries.
    const slotCounts = new Map<string, number>();
    mappings.forEach((m) => {
      if (!m.contact_field || REPEATABLE_VALUE_FIELDS.has(m.contact_field)) return;
      const key = `${m.contact_field}#${m.group}`;
      slotCounts.set(key, (slotCounts.get(key) ?? 0) + 1);
    });
    const conflictKeys = new Set<string>();
    slotCounts.forEach((count, key) => {
      if (count > 1) conflictKeys.add(key);
    });
    const mappingKey = (m: ColumnMapping) => `${m.contact_field}#${m.group}`;
    const conflictLabels = Array.from(
      new Set(
        mappings
          .filter((m) => m.contact_field && conflictKeys.has(mappingKey(m)))
          .map((m) => CONTACT_FIELD_LABELS[m.contact_field] || m.contact_field),
      ),
    );

    return (
      <Box>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {t(
            'contacts.import.mapping.description',
            "Match your CSV columns to contact fields. Columns marked 'Ignore' will not be imported.",
          )}
        </Typography>
        {conflictLabels.length > 0 && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            {t(
              'contacts.import.mapping.duplicateWarning',
              'These single-value fields are mapped to more than one column; only the last column will be used: {{fields}}',
              { fields: conflictLabels.join(', ') },
            )}
          </Alert>
        )}
        <TableContainer component={Paper} sx={{ maxHeight: 400 }}>
          <Table stickyHeader size="small">
            <TableHead>
              <TableRow>
                <TableCell>{t('contacts.import.mapping.csvColumn', 'CSV Column')}</TableCell>
                <TableCell>{t('contacts.import.mapping.sampleData', 'Sample')}</TableCell>
                <TableCell>{t('contacts.import.mapping.mapsTo', 'Maps To')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {mappings.map((mapping, index) => {
                const isConflict = !!mapping.contact_field && conflictKeys.has(mappingKey(mapping));
                return (
                  <TableRow key={index}>
                    <TableCell>
                      <Typography variant="body2" fontWeight="medium">
                        {mapping.csv_column}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Typography variant="body2" color="text.secondary" noWrap sx={{ maxWidth: 150 }}>
                        {uploadResponse.sample_data[0]?.[index] || <EmptyValue />}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <FormControl size="small" sx={{ minWidth: 180 }} error={isConflict}>
                        <Select
                          value={mapping.contact_field}
                          onChange={(e) => {
                            handleMappingChange(index, e.target.value);
                          }}
                          displayEmpty
                        >
                          <MenuItem value="">
                            <em>{t('contacts.import.mapping.ignore', '-- Ignore --')}</em>
                          </MenuItem>
                          {IMPORTABLE_CONTACT_FIELDS.map((field) => (
                            <MenuItem key={field} value={field}>
                              {CONTACT_FIELD_LABELS[field] || field}
                            </MenuItem>
                          ))}
                        </Select>
                        {isConflict && (
                          <FormHelperText>
                            {t('contacts.import.mapping.duplicateField', 'Mapped more than once')}
                          </FormHelperText>
                        )}
                      </FormControl>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    );
  };

  // Render preview step
  const renderPreviewStep = () => {
    if (!previewResponse) return null;

    const { toCreate, toUpdate, toSkip } = getSummaryCounts();
    const errorCount = previewResponse.rows.filter((r) => r.validation_errors.length > 0).length;
    const warningCount = previewResponse.rows.filter((r) => r.warnings.length > 0).length;
    const shownRows = sortRows(
      filter ? previewResponse.rows.filter((r) => filter.rows.has(r.row_index)) : previewResponse.rows,
      rowActions,
      sort,
    );
    const chipFilterProps = (key: PreviewFilter) => ({
      variant: filter?.key === key ? ('filled' as const) : ('outlined' as const),
      onClick: () => {
        toggleFilter(key);
      },
      'aria-pressed': filter?.key === key,
    });
    const sortHeader = (key: PreviewSortKey, label: string) => (
      <TableSortLabel
        active={sort.key === key}
        direction={sort.key === key ? sort.direction : 'asc'}
        onClick={() => {
          toggleSort(key);
        }}
      >
        {label}
      </TableSortLabel>
    );

    return (
      <Box>
        {/* Summary; each chip filters the table */}
        <Box sx={{ mb: 1, display: 'flex', gap: 1, flexWrap: 'wrap' }}>
          <Chip
            icon={<CheckCircleIcon />}
            label={t('contacts.import.preview.toCreate', '{{count}} to create', { count: toCreate })}
            color="success"
            {...chipFilterProps('add')}
          />
          <Chip
            icon={<WarningIcon />}
            label={t('contacts.import.preview.toUpdate', '{{count}} to update', { count: toUpdate })}
            color="warning"
            {...chipFilterProps('update')}
          />
          <Chip
            label={t('contacts.import.preview.toSkip', '{{count}} to skip', { count: toSkip })}
            {...chipFilterProps('skip')}
          />
          {errorCount > 0 && (
            <Chip
              icon={<ErrorIcon />}
              label={t('contacts.import.preview.errors', '{{count}} with errors', { count: errorCount })}
              color="error"
              {...chipFilterProps('errors')}
            />
          )}
          {warningCount > 0 && (
            <Chip
              icon={<ReportProblemOutlinedIcon />}
              label={t('contacts.import.preview.withWarnings', '{{count}} with warnings', { count: warningCount })}
              color="warning"
              {...chipFilterProps('warnings')}
            />
          )}
        </Box>
        <Box sx={{ mb: 2, display: 'flex', gap: 2, alignItems: 'center', flexWrap: 'wrap' }}>
          <Typography variant="body2" color="text.secondary">
            {filter
              ? t('contacts.import.preview.showing', 'Showing {{shown}} of {{total}} rows', {
                  shown: shownRows.length,
                  total: previewResponse.rows.length,
                })
              : t('contacts.import.preview.filterHint', 'Click a count to show only those rows.')}
          </Typography>
          <FormControl size="small" sx={{ minWidth: 220 }}>
            <Select
              displayEmpty
              value=""
              onChange={(e) => {
                applyActionToRows(shownRows, e.target.value);
              }}
              inputProps={{ 'aria-label': t('contacts.import.preview.setAll', 'Set action for all shown rows') }}
            >
              <MenuItem value="" disabled>
                {t('contacts.import.preview.setAll', 'Set action for all shown rows')}
              </MenuItem>
              <MenuItem value="skip">{t('contacts.import.preview.actionSkip', 'Skip')}</MenuItem>
              <MenuItem value="add">{t('contacts.import.preview.actionAdd', 'Add as New')}</MenuItem>
              <MenuItem value="update">{t('contacts.import.preview.actionUpdate', 'Update Existing')}</MenuItem>
            </Select>
          </FormControl>
        </Box>

        {/* Preview table */}
        <TableContainer component={Paper} sx={{ maxHeight: 350 }}>
          <Table stickyHeader size="small">
            <TableHead>
              <TableRow>
                <TableCell width={50}>{sortHeader('row', t('contacts.import.preview.row', 'Row'))}</TableCell>
                <TableCell>{sortHeader('firstname', t('contacts.add.firstname', 'First Name'))}</TableCell>
                <TableCell>{sortHeader('lastname', t('contacts.add.lastname', 'Last Name'))}</TableCell>
                <TableCell>{sortHeader('email', t('contacts.add.email', 'Email'))}</TableCell>
                <TableCell width={120}>{sortHeader('status', t('contacts.import.preview.status', 'Status'))}</TableCell>
                <TableCell width={160}>{sortHeader('action', t('contacts.import.preview.action', 'Action'))}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {shownRows.map((row) => (
                <TableRow key={row.row_index}>
                  <TableCell>{row.row_index + 1}</TableCell>
                  <TableCell>{row.parsed_contact['firstname'] || <EmptyValue />}</TableCell>
                  <TableCell>{row.parsed_contact['lastname'] || <EmptyValue />}</TableCell>
                  <TableCell>{row.parsed_contact['email'] || <EmptyValue />}</TableCell>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      {row.validation_errors.length > 0 ? (
                        <Tooltip title={row.validation_errors.join(', ')}>
                          <Chip
                            icon={<ErrorIcon />}
                            label={t('contacts.import.preview.error')}
                            size="small"
                            color="error"
                          />
                        </Tooltip>
                      ) : row.duplicate_match?.existing_deleted ? (
                        <Tooltip
                          title={t(
                            'contacts.import.preview.restoreOf',
                            'Previously deleted: {{name}}. Importing will restore it.',
                            {
                              name: `${row.duplicate_match.existing_firstname} ${row.duplicate_match.existing_lastname}`,
                            },
                          )}
                        >
                          <Chip
                            icon={<RestoreFromTrashIcon />}
                            label={t('contacts.import.preview.restoreStatus', 'Deleted contact')}
                            size="small"
                            color="info"
                          />
                        </Tooltip>
                      ) : row.duplicate_match ? (
                        <Tooltip
                          title={t('contacts.import.preview.duplicateOf', 'Matches: {{name}} ({{reason}})', {
                            name: `${row.duplicate_match.existing_firstname} ${row.duplicate_match.existing_lastname}`,
                            reason: t(
                              `contacts.import.preview.matchReason.${row.duplicate_match.match_reason}`,
                              row.duplicate_match.match_reason,
                            ),
                          })}
                        >
                          <Chip
                            icon={<WarningIcon />}
                            label={t('contacts.import.preview.duplicateStatus')}
                            size="small"
                            color="warning"
                          />
                        </Tooltip>
                      ) : (
                        <Chip
                          icon={<CheckCircleIcon />}
                          label={t('contacts.import.preview.valid')}
                          size="small"
                          color="success"
                        />
                      )}
                      {row.warnings.length > 0 && (
                        <Tooltip
                          title={t('contacts.import.preview.warnings', 'Not imported: {{list}}', {
                            list: row.warnings.join(', '),
                          })}
                        >
                          <ReportProblemOutlinedIcon
                            fontSize="small"
                            color="warning"
                            aria-label={t('contacts.import.preview.warnings', 'Not imported: {{list}}', {
                              list: row.warnings.join(', '),
                            })}
                          />
                        </Tooltip>
                      )}
                    </Box>
                  </TableCell>
                  <TableCell>
                    <FormControl size="small" fullWidth>
                      <Select
                        value={rowActions.get(row.row_index) || 'skip'}
                        onChange={(e) => {
                          handleRowActionChange(row.row_index, e.target.value);
                        }}
                        disabled={row.validation_errors.length > 0}
                      >
                        <MenuItem value="skip">{t('contacts.import.preview.actionSkip', 'Skip')}</MenuItem>
                        <MenuItem value="add">{t('contacts.import.preview.actionAdd', 'Add as New')}</MenuItem>
                        {row.duplicate_match && (
                          <MenuItem value="update">
                            {row.duplicate_match.existing_deleted
                              ? t('contacts.import.preview.actionRestore', 'Restore Deleted Contact')
                              : t('contacts.import.preview.actionUpdate', 'Update Existing')}
                          </MenuItem>
                        )}
                      </Select>
                    </FormControl>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>
    );
  };

  // Render result step
  const renderPhotoProgress = (queued: number) => {
    const status = photoStatus ?? { phase: 'importing_photos', photos_done: 0, photos_total: queued, photos_saved: 0 };
    if (status.phase === 'done') {
      return (
        <Typography>
          {t('contacts.import.result.photosLoaded', 'Photos loaded: {{saved}} of {{total}}', {
            saved: status.photos_saved,
            total: status.photos_total,
          })}
        </Typography>
      );
    }
    return (
      <Box>
        <Typography>
          {t('contacts.import.result.photosLoading', 'Loading photos: {{done}} of {{total}}', {
            done: status.photos_done,
            total: status.photos_total,
          })}
        </Typography>
        <LinearProgress variant="determinate" value={(status.photos_done / status.photos_total) * 100} sx={{ my: 1 }} />
        <Typography variant="body2" color="text.secondary">
          {t(
            'contacts.import.result.photosKeepLoading',
            'You can close this dialog. The photos keep loading on the server.',
          )}
        </Typography>
      </Box>
    );
  };

  const renderResultStep = () => {
    if (!importResult) return null;
    const skippedRows = importResult.skipped_rows;

    return (
      <Box sx={{ py: 2 }}>
        <Alert severity="success" sx={{ mb: 2 }}>
          {t('contacts.import.result.title', 'Import Complete')}
        </Alert>

        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          <Typography>
            <CheckCircleIcon color="success" sx={{ verticalAlign: 'middle', mr: 1 }} />
            {t('contacts.import.result.created', '{{count}} contacts created', {
              count: importResult.created,
            })}
          </Typography>
          <Typography>
            <WarningIcon color="warning" sx={{ verticalAlign: 'middle', mr: 1 }} />
            {t('contacts.import.result.updated', '{{count}} contacts updated', {
              count: importResult.updated,
            })}
          </Typography>
          <Typography color="text.secondary">
            {t('contacts.import.result.skipped', '{{count}} rows skipped', {
              count: importResult.skipped,
            })}
          </Typography>
          {importResult.photos_queued > 0 && !photoPollFailed && renderPhotoProgress(importResult.photos_queued)}
        </Box>

        {skippedRows.length > 0 && (
          <TableContainer component={Paper} sx={{ mt: 2, maxHeight: 250 }}>
            <Table stickyHeader size="small" aria-label={t('contacts.import.result.skippedRows', 'Skipped rows')}>
              <TableHead>
                <TableRow>
                  <TableCell width={50}>{t('contacts.import.preview.row', 'Row')}</TableCell>
                  <TableCell>{t('contacts.import.result.name', 'Name')}</TableCell>
                  <TableCell>{t('contacts.import.result.reason', 'Reason')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {skippedRows.map((r) => (
                  <TableRow key={r.row}>
                    <TableCell>{r.row}</TableCell>
                    <TableCell>{r.name || <EmptyValue />}</TableCell>
                    <TableCell sx={{ color: r.reason === 'skipped' ? 'text.secondary' : 'error.main' }}>
                      {t(`contacts.import.result.reasons.${r.reason}`, r.reason)}
                      {r.detail ? `: ${r.detail}` : ''}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Box>
    );
  };

  // Render current step content
  const renderStepContent = () => {
    switch (step) {
      case 'upload':
        return renderUploadStep();
      case 'mapping':
        return renderMappingStep();
      case 'preview':
        return renderPreviewStep();
      case 'result':
        return renderResultStep();
      default:
        return null;
    }
  };

  // Render action buttons
  const renderActions = () => {
    switch (step) {
      case 'upload':
        return <Button onClick={handleClose}>{t('common.cancel', 'Cancel')}</Button>;
      case 'mapping':
        return (
          <>
            <Button
              onClick={() => {
                setStep('upload');
                setActiveStep(0);
              }}
            >
              {t('common.back', 'Back')}
            </Button>
            <Button variant="contained" onClick={handleGeneratePreview} disabled={loading}>
              {t('common.continue', 'Continue')}
            </Button>
          </>
        );
      case 'preview':
        return (
          <>
            <Button
              onClick={() => {
                if (importType === 'vcf') {
                  // VCF goes back to upload (no mapping step)
                  setStep('upload');
                  setActiveStep(0);
                  setPreviewResponse(null);
                  setRowActions(new Map());
                } else {
                  // CSV goes back to mapping
                  setStep('mapping');
                  setActiveStep(1);
                }
              }}
            >
              {t('common.back', 'Back')}
            </Button>
            <Button onClick={handleClose}>{t('common.cancel', 'Cancel')}</Button>
            <Button variant="contained" onClick={handleConfirmImport} disabled={loading}>
              {t('contacts.import.button', 'Import')}
            </Button>
          </>
        );
      case 'result':
        return (
          <Button variant="contained" onClick={handleClose}>
            {t('contacts.import.result.done', 'Done')}
          </Button>
        );
      default:
        return null;
    }
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          {t('contacts.import.title', 'Import Contacts')}
          <IconButton onClick={handleClose} size="small">
            <CloseIcon />
          </IconButton>
        </Box>
      </DialogTitle>

      <DialogContent dividers>
        {/* Stepper - different steps for CSV vs VCF */}
        <Stepper activeStep={activeStep} sx={{ mb: 3 }}>
          {(importType === 'vcf' ? VCF_STEP_KEYS : CSV_STEP_KEYS).map((key) => (
            <Step key={key}>
              <StepLabel>{t(`contacts.import.steps.${key}`)}</StepLabel>
            </Step>
          ))}
        </Stepper>

        {/* Loading indicator */}
        {loading && <LinearProgress sx={{ mb: 2 }} />}

        {/* Error alert */}
        {error && (
          <Alert
            severity="error"
            sx={{ mb: 2 }}
            onClose={() => {
              setError(null);
            }}
          >
            {error}
          </Alert>
        )}

        {/* Step content */}
        {renderStepContent()}
      </DialogContent>

      <DialogActions>{renderActions()}</DialogActions>
    </AppDialog>
  );
}
