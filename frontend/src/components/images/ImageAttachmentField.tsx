import { useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, ButtonBase, CircularProgress, IconButton, Typography } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import CloseIcon from '@mui/icons-material/Close';
import ReplayIcon from '@mui/icons-material/Replay';
import ImageIcon from '@mui/icons-material/Image';
import { useImageInput } from '../../hooks/useImageInput';
import type { useImageUploads } from '../../hooks/useImageUploads';

interface ImageAttachmentFieldProps {
  uploads: ReturnType<typeof useImageUploads>;
  disabled?: boolean;
}

const TILE = 72;

export default function ImageAttachmentField({ uploads, disabled = false }: ImageAttachmentFieldProps) {
  const { t } = useTranslation();
  const { items, addFiles, remove, retry, move } = uploads;
  const [rejected, setRejected] = useState(false);
  const [dragFrom, setDragFrom] = useState<number | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const { dropProps } = useImageInput({
    enabled: !disabled,
    onFiles: (files) => {
      setRejected(false);
      addFiles(files);
    },
    onReject: () => {
      setRejected(true);
    },
  });

  const onKeyDown = (event: KeyboardEvent, index: number) => {
    if (!event.altKey || (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight')) return;
    event.preventDefault();
    move(index, index + (event.key === 'ArrowLeft' ? -1 : 1));
  };

  return (
    <Box {...dropProps}>
      <Box
        role="list"
        sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, alignItems: 'center' }}
        aria-describedby={items.length > 1 ? 'image-reorder-hint' : undefined}
      >
        {items.map((item, index) => (
          <Box
            key={item.key}
            role="listitem"
            draggable={!disabled && item.status === 'done'}
            onDragStart={() => {
              setDragFrom(index);
            }}
            onDragOver={(e) => {
              if (dragFrom !== null) e.preventDefault();
            }}
            onDrop={(e) => {
              if (dragFrom === null) return;
              e.preventDefault();
              e.stopPropagation();
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
                borderColor: item.status === 'error' ? 'error.main' : 'divider',
              }}
            >
              {item.previewUrl ? (
                <Box
                  component="img"
                  src={item.previewUrl}
                  alt=""
                  sx={{ width: '100%', height: '100%', objectFit: 'cover', opacity: item.status === 'done' ? 1 : 0.5 }}
                />
              ) : (
                <ImageIcon color="disabled" />
              )}
            </ButtonBase>
            {item.status === 'uploading' && (
              <CircularProgress
                variant={item.progress > 0 ? 'determinate' : 'indeterminate'}
                value={item.progress * 100}
                size={32}
                sx={{ position: 'absolute', top: TILE / 2 - 16, left: TILE / 2 - 16 }}
              />
            )}
            {item.status === 'error' && (
              <IconButton
                size="small"
                aria-label={t('images.retry')}
                disabled={item.errorKind === 'tooLarge'}
                onClick={() => {
                  retry(item.key);
                }}
                sx={{ position: 'absolute', top: TILE / 2 - 16, left: TILE / 2 - 16, bgcolor: 'background.paper' }}
              >
                <ReplayIcon fontSize="small" />
              </IconButton>
            )}
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
        <ButtonBase
          component="label"
          disabled={disabled}
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
          <input
            ref={input}
            type="file"
            accept="image/*"
            multiple
            hidden
            aria-label={t('images.addImage')}
            onChange={(e) => {
              setRejected(false);
              addFiles(Array.from(e.target.files ?? []));
              e.target.value = '';
            }}
          />
          <AddPhotoAlternateIcon fontSize="small" />
          <Typography variant="caption">{t('images.addImage')}</Typography>
        </ButtonBase>
      </Box>
      {items.map((item, index) =>
        item.status === 'error' ? (
          <Typography key={item.key} variant="caption" color="error" role="alert" sx={{ display: 'block', mt: 0.5 }}>
            {index + 1}: {item.errorKind === 'tooLarge' ? t('images.tooLarge') : item.error || t('images.uploadFailed')}
          </Typography>
        ) : null,
      )}
      {rejected && (
        <Typography variant="caption" color="error" role="alert" sx={{ display: 'block', mt: 0.5 }}>
          {t('images.notAnImage')}
        </Typography>
      )}
      {items.length > 1 && (
        <Typography id="image-reorder-hint" variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
          {t('images.reorderHint')}
        </Typography>
      )}
    </Box>
  );
}
