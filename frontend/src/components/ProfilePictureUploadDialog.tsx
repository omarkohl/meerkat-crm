import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  CircularProgress,
  Alert,
  TextField,
  Divider,
} from '@mui/material';
import AppDialog from './AppDialog';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import LinkIcon from '@mui/icons-material/Link';
import { handleError, getErrorMessage } from '../utils/errorHandler';
import { useImageInput } from '../hooks/useImageInput';
import FaceCropper from './images/FaceCropper';
import { toServerCrop, type Area, type Size } from '../utils/faceCrop';
import { API_BASE_URL, apiFetch, parseErrorResponse } from '../api/client';
import { MAX_IMAGE_BYTES, uploadImage, type ImageRef } from '../api/images';
import { apiUrl } from '../utils/thumbnail';

interface ProfilePictureUploadDialogProps {
  open: boolean;
  onClose: () => void;
  onSave: (image: ImageRef, crop: { x: number; y: number; size: number } | null) => Promise<void>;
  initialFile?: File | null;
  // False when the photo is only added to the gallery: no crop is asked for.
  crop?: boolean;
}

const isMac = /Mac|iPhone|iPad/.test(navigator.userAgent);

export default function ProfilePictureUploadDialog({
  open,
  onClose,
  onSave,
  initialFile,
  crop = true,
}: ProfilePictureUploadDialogProps) {
  const { t } = useTranslation();
  const [image, setImage] = useState<ImageRef | null>(null);
  const [picked, setPicked] = useState<{ area: Area; natural: Size } | null>(null);
  const [uploading, setUploading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [imageUrl, setImageUrl] = useState('');
  const [fetchingUrl, setFetchingUrl] = useState(false);

  // The server converts the file, so the crop is drawn on its copy and HEIC works in every browser.
  const loadFile = async (chosen: File) => {
    setError(null);

    if (chosen.size > MAX_IMAGE_BYTES) {
      setError(t('images.tooLarge'));
      return;
    }

    setUploading(true);
    try {
      const uploaded = await uploadImage(chosen, () => undefined).promise;
      setImage(uploaded);
      setPicked(null);
    } catch (err) {
      handleError(err, { operation: 'uploading profile picture' });
      setError(getErrorMessage(err));
    } finally {
      setUploading(false);
    }
  };

  useEffect(() => {
    if (open && initialFile) void loadFile(initialFile);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- load once per opening, not per render
  }, [open, initialFile]);

  const { dropProps } = useImageInput({
    enabled: open && !uploading && !saving,
    onFiles: ([file]) => {
      if (file) void loadFile(file);
    },
    onReject: () => {
      setError(t('profilePicture.invalidFileType'));
    },
  });

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    if (!file.type.startsWith('image/')) {
      setError(t('profilePicture.invalidFileType'));
      return;
    }
    void loadFile(file);
  };

  const handleFetchFromUrl = async () => {
    if (!imageUrl.trim()) return;

    setError(null);
    setFetchingUrl(true);

    try {
      const proxyUrl = `${API_BASE_URL}/proxy/image?url=${encodeURIComponent(imageUrl.trim())}`;

      const response = await apiFetch(proxyUrl);

      if (!response.ok) {
        throw await parseErrorResponse(response);
      }

      const contentType = response.headers.get('content-type');
      if (!contentType || !contentType.startsWith('image/')) {
        throw new Error(t('profilePicture.invalidFileType'));
      }

      const blob = await response.blob();

      if (blob.size > MAX_IMAGE_BYTES) {
        throw new Error(t('images.tooLarge'));
      }

      await loadFile(new File([blob], 'image', { type: blob.type }));
      setImageUrl('');
    } catch (err) {
      handleError(err, { operation: 'fetching image from URL' });
      setError(getErrorMessage(err));
    } finally {
      setFetchingUrl(false);
    }
  };

  const handleSave = async () => {
    if (!image || (crop && !picked)) return;

    setSaving(true);
    setError(null);

    try {
      await onSave(image, crop && picked ? toServerCrop(picked.area, picked.natural, image) : null);
      handleClose();
    } catch (err) {
      handleError(err, { operation: 'saving profile picture' });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const handleClose = () => {
    setImage(null);
    setPicked(null);
    setError(null);
    setImageUrl('');
    setFetchingUrl(false);
    onClose();
  };

  return (
    <AppDialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>{t('profilePicture.title')}</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}

        {!image ? (
          // File selection view
          <Box>
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
              {...dropProps}
            >
              <input type="file" accept="image/*" onChange={handleFileSelect} style={{ display: 'none' }} />
              {uploading ? (
                <CircularProgress sx={{ mb: 1 }} />
              ) : (
                <CloudUploadIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 1 }} />
              )}
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
                disabled={fetchingUrl || uploading}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleFetchFromUrl();
                  }
                }}
                InputProps={{
                  startAdornment: <LinkIcon sx={{ mr: 1, color: 'text.secondary' }} />,
                }}
              />
              <Button
                variant="outlined"
                onClick={handleFetchFromUrl}
                disabled={!imageUrl.trim() || fetchingUrl || uploading}
                sx={{ minWidth: 100 }}
              >
                {fetchingUrl || uploading ? <CircularProgress size={20} /> : t('profilePicture.fetch')}
              </Button>
            </Box>
          </Box>
        ) : (
          // Cropping view
          <Box>
            {crop ? (
              <FaceCropper
                src={apiUrl(image.url)}
                onChange={(area, natural) => {
                  setPicked({ area, natural });
                }}
              />
            ) : (
              <Box
                component="img"
                src={apiUrl(image.thumb_url)}
                alt=""
                sx={{ display: 'block', maxWidth: '100%', maxHeight: 360, mx: 'auto', borderRadius: 1 }}
              />
            )}

            {/* Change image button */}
            <Box sx={{ display: 'flex', justifyContent: 'center', mt: 1 }}>
              <Button component="label" size="small" variant="text">
                {t('profilePicture.changeImage')}
                <input type="file" accept="image/*" onChange={handleFileSelect} style={{ display: 'none' }} />
              </Button>
            </Box>
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button
          onClick={handleSave}
          variant="contained"
          disabled={!image || (crop && !picked) || saving}
          startIcon={saving ? <CircularProgress size={20} /> : undefined}
        >
          {saving ? t('profilePicture.uploading') : t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
