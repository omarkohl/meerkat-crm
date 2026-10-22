import { thumbnailSrc } from '../../utils/thumbnail';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, ButtonBase, IconButton, Menu, MenuItem } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import StarIcon from '@mui/icons-material/Star';
import type { ImageRef } from '../../api/images';

interface ImageGridProps {
  images: ImageRef[];
  mainId: number | null;
  onOpen: (index: number) => void;
  onAdd: () => void;
  onFrame: (image: ImageRef) => void;
  onDelete: (image: ImageRef) => void;
}

const TILE = 96;

export default function ImageGrid({ images, mainId, onOpen, onAdd, onFrame, onDelete }: ImageGridProps) {
  const { t } = useTranslation();
  const [menu, setMenu] = useState<{ anchor: HTMLElement; image: ImageRef } | null>(null);

  const choose = (action: (image: ImageRef) => void) => {
    if (menu) action(menu.image);
    setMenu(null);
  };

  return (
    <Box role="list" sx={{ display: 'flex', flexWrap: 'wrap', gap: 1 }}>
      {images.map((image, index) => (
        <Box key={image.id} role="listitem" sx={{ position: 'relative', width: TILE, height: TILE }}>
          <ButtonBase
            aria-label={t('images.open', { n: index + 1, total: images.length })}
            onClick={() => {
              onOpen(index);
            }}
            sx={{ width: TILE, height: TILE, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
          >
            <Box
              component="img"
              src={thumbnailSrc(image.thumb_url)}
              alt=""
              loading="lazy"
              sx={{ width: TILE, height: TILE, objectFit: 'cover', display: 'block' }}
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
      <ButtonBase
        aria-label={t('images.addPhoto')}
        onClick={onAdd}
        sx={{
          width: TILE,
          height: TILE,
          borderRadius: 1,
          border: 1,
          borderStyle: 'dashed',
          borderColor: 'divider',
          color: 'text.secondary',
        }}
      >
        <AddPhotoAlternateIcon />
      </ButtonBase>
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
