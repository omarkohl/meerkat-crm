import { apiUrl } from '../../utils/thumbnail';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, CircularProgress, DialogActions, DialogContent, DialogTitle } from '@mui/material';
import AppDialog from '../AppDialog';
import FaceCropper from './FaceCropper';
import type { ImageRef } from '../../api/images';
import { handleError, getErrorMessage } from '../../utils/errorHandler';
import { toServerCrop, type Area, type Size } from '../../utils/faceCrop';

interface FaceCropDialogProps {
  image: ImageRef | null;
  onClose: () => void;
  onSave: (image: ImageRef, crop: { x: number; y: number; size: number }) => Promise<void>;
}

export default function FaceCropDialog({ image, onClose, onSave }: FaceCropDialogProps) {
  const { t } = useTranslation();
  const [picked, setPicked] = useState<{ area: Area; natural: Size } | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    setPicked(null);
    setError(null);
    onClose();
  };

  const save = async () => {
    if (!image || !picked) return;
    setSaving(true);
    setError(null);
    try {
      await onSave(image, toServerCrop(picked.area, picked.natural, image));
      close();
    } catch (err) {
      handleError(err, { operation: 'saving face crop' });
      setError(getErrorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  const crop = image?.crop;
  return (
    <AppDialog open={image !== null} onClose={close} maxWidth="sm" fullWidth>
      <DialogTitle>{t('profilePicture.adjustCrop')}</DialogTitle>
      <DialogContent>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error}
          </Alert>
        )}
        {image && (
          <FaceCropper
            src={apiUrl(image.url)}
            initialArea={crop ? { x: crop.x, y: crop.y, width: crop.size, height: crop.size } : undefined}
            onChange={(area, natural) => {
              setPicked({ area, natural });
            }}
          />
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={close} disabled={saving}>
          {t('common.cancel')}
        </Button>
        <Button
          variant="contained"
          onClick={() => {
            void save();
          }}
          disabled={!picked || saving}
          startIcon={saving ? <CircularProgress size={20} /> : undefined}
        >
          {t('common.save')}
        </Button>
      </DialogActions>
    </AppDialog>
  );
}
