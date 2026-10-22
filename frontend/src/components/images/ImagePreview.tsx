import { thumbnailSrc } from '../../utils/thumbnail';
import { useTranslation } from 'react-i18next';
import { Box, ButtonBase, Typography } from '@mui/material';
import type { ImageRef } from '../../api/images';

// The rest becomes a "+N" badge on the last preview.
const VISIBLE = 3;

interface ImagePreviewProps {
  images: ImageRef[];
  onOpen: (index: number) => void;
  size?: number;
}

export default function ImagePreview({ images, onOpen, size = 64 }: ImagePreviewProps) {
  const { t } = useTranslation();
  if (images.length === 0) return null;
  const shown = images.slice(0, VISIBLE);
  const extra = images.length - shown.length;

  return (
    <Box sx={{ display: 'flex', gap: 1, mt: 1 }}>
      {shown.map((image, index) => (
        <ButtonBase
          key={image.id}
          aria-label={t('images.open', { n: index + 1, total: images.length })}
          onClick={(e) => {
            e.stopPropagation();
            onOpen(index);
          }}
          sx={{ position: 'relative', width: size, height: size, borderRadius: 1, overflow: 'hidden', flex: 'none' }}
        >
          <Box
            component="img"
            src={thumbnailSrc(image.thumb_url)}
            alt=""
            loading="lazy"
            width={size}
            height={size}
            sx={{ width: size, height: size, objectFit: 'cover', display: 'block' }}
          />
          {extra > 0 && index === shown.length - 1 && (
            <Typography
              variant="caption"
              sx={{
                position: 'absolute',
                right: 2,
                bottom: 2,
                px: 0.5,
                borderRadius: 0.5,
                bgcolor: 'rgba(0,0,0,0.65)',
                color: 'common.white',
              }}
            >
              +{extra}
            </Typography>
          )}
        </ButtonBase>
      ))}
    </Box>
  );
}
