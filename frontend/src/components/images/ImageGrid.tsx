import { thumbnailSrc } from '../../utils/thumbnail';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, ButtonBase, IconButton, Menu, MenuItem } from '@mui/material';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import StarIcon from '@mui/icons-material/Star';
import type { ImageRef } from '../../api/images';
import { PHOTO_GRID } from './photoGrid';

interface ImageGridProps {
  images: ImageRef[];
  mainId: number | null;
  onOpen: (index: number) => void;
  onFrame: (image: ImageRef) => void;
  onDelete: (image: ImageRef) => void;
}

export default function ImageGrid({ images, mainId, onOpen, onFrame, onDelete }: ImageGridProps) {
  const { t } = useTranslation();
  const [menu, setMenu] = useState<{ anchor: HTMLElement; image: ImageRef } | null>(null);

  const choose = (action: (image: ImageRef) => void) => {
    if (menu) action(menu.image);
    setMenu(null);
  };

  return (
    <Box role="list" sx={{ display: 'grid', gridTemplateColumns: PHOTO_GRID, gap: 1.5 }}>
      {images.map((image, index) => (
        <Box key={image.id} role="listitem" sx={{ position: 'relative', aspectRatio: '1' }}>
          <ButtonBase
            aria-label={t('images.open', { n: index + 1, total: images.length })}
            onClick={() => {
              onOpen(index);
            }}
            sx={{ width: '100%', height: '100%', borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
          >
            <Box
              component="img"
              src={thumbnailSrc(image.thumb_url)}
              alt=""
              loading="lazy"
              sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
            />
          </ButtonBase>
          {image.id === mainId && (
            <StarIcon
              titleAccess={t('images.mainImage')}
              sx={{
                position: 'absolute',
                left: 4,
                top: 4,
                fontSize: 18,
                color: 'warning.main',
                filter: 'drop-shadow(0 0 2px #000)',
              }}
            />
          )}
          <IconButton
            size="small"
            aria-label={t('images.actions', { n: index + 1 })}
            onClick={(e) => {
              setMenu({ anchor: e.currentTarget, image });
            }}
            sx={{ position: 'absolute', right: 2, top: 2, bgcolor: 'rgba(0,0,0,0.55)', color: 'common.white', p: 0.25 }}
          >
            <MoreVertIcon fontSize="small" />
          </IconButton>
        </Box>
      ))}
      <Menu
        open={menu !== null}
        anchorEl={menu?.anchor}
        onClose={() => {
          setMenu(null);
        }}
      >
        <MenuItem
          onClick={() => {
            choose(onFrame);
          }}
        >
          {menu?.image.id === mainId ? t('images.adjustCrop') : t('images.setMain')}
        </MenuItem>
        <MenuItem
          onClick={() => {
            choose(onDelete);
          }}
        >
          {t('images.delete')}
        </MenuItem>
      </Menu>
    </Box>
  );
}
