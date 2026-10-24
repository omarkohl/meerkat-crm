import { useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Button, ButtonBase, IconButton, Typography } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import CloseIcon from '@mui/icons-material/Close';
import AddPhotosDialog from './AddPhotosDialog';
import type { useAttachedImages } from '../../hooks/useAttachedImages';

interface ImageAttachmentFieldProps {
  attached: ReturnType<typeof useAttachedImages>;
  disabled?: boolean;
}

const TILE = 72;

export default function ImageAttachmentField({ attached, disabled = false }: ImageAttachmentFieldProps) {
  const { t } = useTranslation();
  const { items, add, remove, move } = attached;
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  const [adding, setAdding] = useState(false);

  const onKeyDown = (event: KeyboardEvent, index: number) => {
    if (!event.altKey || (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')) return;
    event.preventDefault();
    move(index, index + (event.key === 'ArrowLeft' ? -1 : 1));
  };

  return (
    <Box>
      {items.length > 0 && (
        <Box
          role="list"
          sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, mb: 1 }}
          aria-describedby={items.length > 1 ? 'image-reorder-hint' : undefined}
        >
          {items.map((item, index) => (
            <Box
              key={item.key}
              role="listitem"
              draggable={!disabled}
              onDragStart={() => {
                setDragFrom(index);
              }}
              onDragOver={(e) => {
                if (dragFrom !== null) e.preventDefault();
              }}
              onDrop={(e) => {
                if (dragFrom === null) return;
                e.preventDefault();
                move(dragFrom, index);
                setDragFrom(null);
              }}
              sx={{ position: 'relative', width: TILE, height: TILE, flex: 'none' }}
            >
              <ButtonBase
                aria-label={t('images.imageOf', { n: index + 1, total: items.length })}
                onKeyDown={(e) => {
                  onKeyDown(e, index);
                }}
                sx={{
                  width: TILE,
                  height: TILE,
                  borderRadius: 1,
                  overflow: 'hidden',
                  bgcolor: 'action.hover',
                  border: 1,
                  borderColor: 'divider',
                }}
              >
                <Box
                  component="img"
                  src={item.previewUrl}
                  alt=""
                  sx={{ width: '100%', height: '100%', objectFit: 'cover' }}
                />
              </ButtonBase>
              <IconButton
                size="small"
                aria-label={t('images.removeImage', { n: index + 1 })}
                onClick={() => {
                  remove(item.key);
                }}
                disabled={disabled}
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
        </Box>
      )}
      <Button
        size="small"
        startIcon={<AddPhotoAlternateIcon />}
        disabled={disabled}
        onClick={() => {
          setAdding(true);
        }}
      >
        {t('images.addPhoto')}
      </Button>
      {items.length > 1 && (
        <Typography id="image-reorder-hint" variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
          {t('images.reorderHint')}
        </Typography>
      )}
      <AddPhotosDialog
        open={adding}
        onClose={() => {
          setAdding(false);
        }}
        onSave={(images) => {
          add(images);
          return Promise.resolve();
        }}
      />
    </Box>
  );
}
