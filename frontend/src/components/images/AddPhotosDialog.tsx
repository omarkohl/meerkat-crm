import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  ButtonBase,
  Typography,
  CircularProgress,
  Alert,
  TextField,
  Divider,
  Checkbox,
  FormControlLabel,
  IconButton,
} from '@mui/material';
import AppDialog from '../AppDialog';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import LinkIcon from '@mui/icons-material/Link';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import CloseIcon from '@mui/icons-material/Close';
import ReplayIcon from '@mui/icons-material/Replay';
import ImageIcon from '@mui/icons-material/Image';
import StarIcon from '@mui/icons-material/Star';
import { handleError, getErrorMessage } from '../../utils/errorHandler';
import { useImageInput } from '../../hooks/useImageInput';
import { useImageUploads } from '../../hooks/useImageUploads';
import FaceCropper from './FaceCropper';
import { toServerCrop, type Area, type Crop, type Size } from '../../utils/faceCrop';
import { MAX_IMAGE_BYTES, fetchImageFromUrl, type ImageRef } from '../../api/images';
import { apiUrl } from '../../utils/thumbnail';

interface AddPhotosDialogProps {
  open: boolean;
  onClose: () => void;
  // The crop, when there is one, is for images[0].
  onSave: (images: ImageRef[], crop: Crop | null) => Promise<void>;
  initialFiles?: File[];
  // Shows the "main picture" box with this start value. Without it the photos are only added.
  mainDefault?: boolean;
  mainLocked?: boolean;
}

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent);
const TILE = 72;

export default function AddPhotosDialog({
  open,
  onClose,
  onSave,
  initialFiles,
  mainDefault,
  mainLocked = false,
}: AddPhotosDialogProps) {
  const { t } = useTranslation();
  const uploads = useImageUploads();
  const { items, addFiles, remove, retry, reset } = uploads;
  const [picked, setPicked] = useState<{ id: number; area: Area; natural: Size } | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [imageUrl, setImageUrl] = useState('');
  const [fetchingUrl, setFetchingUrl] = useState(false);
  const [asMain, setAsMain] = useState(mainDefault ?? false);

  // The server converts each file, so the crop is drawn on its copy and HEIC works in every browser.
  const first = uploads.images[0];
  const framed = !asMain || (first !== undefined && picked?.id === first.id);

  useEffect(() => {
    if (open) setAsMain(mainDefault ?? false);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset per opening only
  }, [open]);

  useEffect(() => {
    if (open && initialFiles?.length) addFiles(initialFiles);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- load once per opening, not per render
  }, [open, initialFiles]);

  const { dropProps } = useImageInput({
    enabled: open && !saving,
    onFiles: (files) => {
      setError(null);
      addFiles(files);
    },
    onReject: () => {
      setError(t('profilePicture.invalidFileType'));
    },
  });

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    event.target.value = '';
    const accepted = files.filter((f) => f.type.startsWith('image/'));
    setError(accepted.length < files.length ? t('profilePicture.invalidFileType') : null);
    if (accepted.length > 0) addFiles(accepted);
  };

  const handleFetchFromUrl = async () => {
    if (!imageUrl.trim()) return;

    setError(null);
    setFetchingUrl(true);

    try {
      const blob = await fetchImageFromUrl(imageUrl.trim());
      if (!blob.type.startsWith('image/')) {
        throw new Error(t('profilePicture.invalidFileType'));
      }

      if (blob.size > MAX_IMAGE_BYTES) {
        throw new Error(t('images.tooLarge'));
      }

      addFiles([new File([blob], 'image', { type: blob.type })]);
      setImageUrl('');
    } catch (err) {
      handleError(err, { operation: 'fetching image from URL' });
      setError(getErrorMessage(err));
    } finally {
      setFetchingUrl(false);
    }
  };

  const handleSave = async () => {
    if (!first || !framed) return;

    setSaving(true);
    setError(null);

    try {
      await onSave(uploads.images, asMain && picked ? toServerCrop(picked.area, picked.natural, first) : null);
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'saving photos' });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    reset();
    setPicked(null);
    setError(null);
    setImageUrl('');
    setFetchingUrl(false);
    onClose();
  };

  const fileInput = <input type="file" accept="image/*" multiple onChange={handleFileSelect} hidden />;

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('profilePicture.title')}</DialogTitle>
      <DialogContent {...dropProps}>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {items.length === 0 ? (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              minHeight: 200,
              border: '2px dashed',
              borderColor: 'divider',
              borderRadius: 2,
              p: 4,
              cursor: 'pointer',
              '&:hover': {
                borderColor: 'primary.main',
                bgcolor: 'action.hover',
              },
            }}
            component="label"
          >
            {fileInput}
            <CloudUploadIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 1 }} />
            <Typography variant="h6" color="text.secondary">
              {t('profilePicture.selectImage')}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {t('profilePicture.dragOrClick')}
            </Typography>
            <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
              {t('profilePicture.pasteHint', { shortcut: isMac ? '⌘V' : 'Ctrl+V' })}
            </Typography>
          </Box>
        ) : (
          <Box>
            {asMain &&
              (first ? (
                <FaceCropper
                  key={first.id}
                  src={apiUrl(first.url)}
                  onChange={(area, natural) => {
                    setPicked({ id: first.id, area, natural });
                  }}
                />
              ) : (
                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 200 }}>
                  <CircularProgress />
                </Box>
              ))}
            <Box role="list" sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mt: asMain ? 2 : 0 }}>
              {items.map((item, index) => (
                <Box key={item.key} role="listitem" sx={{ position: 'relative', width: TILE, height: TILE }}>
                  <Box
                    sx={{
                      width: TILE,
                      height: TILE,
                      borderRadius: 1,
                      overflow: 'hidden',
                      bgcolor: 'action.hover',
                      border: 1,
                      borderColor: item.status === 'error' ? 'error.main' : 'divider',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                    }}
                  >
                    {item.previewUrl ? (
                      <Box
                        component="img"
                        src={item.previewUrl}
                        alt=""
                        sx={{
                          width: '100%',
                          height: '100%',
                          objectFit: 'cover',
                          opacity: item.status === 'done' ? 1 : 0.5,
                        }}
                      />
                    ) : (
                      <ImageIcon color="disabled" />
                    )}
                  </Box>
                  {asMain && item.image && item.image.id === first?.id && (
                    <StarIcon
                      fontSize="small"
                      sx={{ position: 'absolute', left: 4, bottom: 4, color: 'warning.main' }}
                      aria-label={t('images.mainImage')}
                    />
                  )}
                  {item.status === 'uploading' && (
                    <CircularProgress
                      variant={item.progress > 0 ? 'determinate' : 'indeterminate'}
                      value={item.progress * 100}
                      size={32}
                      sx={{ position: 'absolute', top: TILE / 2 - 16, left: TILE / 2 - 16 }}
                    />
                  )}
                  {item.status === 'error' && item.errorKind !== 'tooLarge' && (
                    <IconButton
                      size="small"
                      aria-label={t('images.retry')}
                      onClick={() => {
                        retry(item.key);
                      }}
                      sx={{
                        position: 'absolute',
                        top: TILE / 2 - 16,
                        left: TILE / 2 - 16,
                        bgcolor: 'background.paper',
                      }}
                    >
                      <ReplayIcon fontSize="small" />
                    </IconButton>
                  )}
                  <IconButton
                    size="small"
                    aria-label={t('images.removeImage', { n: index + 1 })}
                    disabled={saving}
                    onClick={() => {
                      remove(item.key);
                    }}
                    sx={{
                      position: 'absolute',
                      top: -8,
                      right: -8,
                      width: 22,
                      height: 22,
                      bgcolor: 'background.paper',
                      border: 1,
                      borderColor: 'divider',
                      '&:hover': { bgcolor: 'background.paper' },
                    }}
                  >
                    <CloseIcon sx={{ fontSize: 14 }} />
                  </IconButton>
                </Box>
              ))}
              <ButtonBase
                component="label"
                disabled={saving}
                sx={{
                  width: TILE,
                  height: TILE,
                  borderRadius: 1,
                  border: 1,
                  borderStyle: 'dashed',
                  borderColor: 'divider',
                  flexDirection: 'column',
                  gap: 0.25,
                  color: 'text.secondary',
                  '&:hover': { borderColor: 'primary.main', color: 'primary.main' },
                }}
              >
                {fileInput}
                <AddPhotoAlternateIcon fontSize="small" />
                <Typography variant="caption">{t('images.addMore')}</Typography>
              </ButtonBase>
            </Box>
            {items.map((item, index) =>
              item.status === 'error' ? (
                <Typography
                  key={item.key}
                  variant="caption"
                  color="error"
                  role="alert"
                  sx={{ display: 'block', mt: 0.5 }}
                >
                  {index + 1}:{' '}
                  {item.errorKind === 'tooLarge' ? t('images.tooLarge') : item.error || t('images.uploadFailed')}
                </Typography>
              ) : null,
            )}
          </Box>
        )}

        <Divider sx={{ my: 3 }}>
          <Typography variant="body2" color="text.secondary">
            {t('profilePicture.or')}
          </Typography>
        </Divider>

        <Box sx={{ display: 'flex', gap: 1, alignItems: 'flex-start' }}>
          <TextField
            fullWidth
            size="small"
            placeholder={t('profilePicture.urlPlaceholder')}
            value={imageUrl}
            onChange={(e) => {
              setImageUrl(e.target.value);
            }}
            disabled={fetchingUrl}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void handleFetchFromUrl();
              }
            }}
            slotProps={{
              input: { startAdornment: <LinkIcon sx={{ mr: 1, color: 'text.secondary' }} /> },
            }}
          />
          <Button
            variant="outlined"
            onClick={() => {
              void handleFetchFromUrl();
            }}
            disabled={!imageUrl.trim() || fetchingUrl}
            sx={{ minWidth: 100 }}
          >
            {fetchingUrl ? <CircularProgress size={20} /> : t('profilePicture.fetch')}
          </Button>
        </Box>

        {mainDefault !== undefined && (
          <FormControlLabel
            sx={{ mt: 1 }}
            control={
              <Checkbox
                checked={asMain}
                disabled={saving || mainLocked}
                onChange={(e) => {
                  setAsMain(e.target.checked);
                }}
              />
            }
            label={t('profilePicture.useAsMain')}
          />
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={() => {
            void handleSave();
          }}
          variant="contained"
          disabled={!first || uploads.busy || uploads.failed || !framed || saving}
          startIcon={saving ? <CircularProgress size={20} /> : undefined}
        >
          {saving ? t('profilePicture.uploading') : t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
