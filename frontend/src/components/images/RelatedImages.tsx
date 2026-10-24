import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { Box, Button, ButtonBase, Link, Typography } from '@mui/material';
import type { RelatedImage } from '../../api/images';
import { useDateFormat } from '../../DateFormatProvider';
import { thumbnailSrc } from '../../utils/thumbnail';
import { PHOTO_GRID } from './photoGrid';

interface RelatedImagesProps {
  items: RelatedImage[];
  total: number;
  onOpen: (index: number) => void;
  onMore: () => void;
}

export default function RelatedImages({ items, total, onOpen, onMore }: RelatedImagesProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();

  return (
    <Box>
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        {t('images.related')}
      </Typography>
      <Box role="list" sx={{ display: 'grid', gridTemplateColumns: PHOTO_GRID, gap: 1.5 }}>
        {items.map((item, index) => {
          const isNote = item.source.type === 'note';
          // Notes of a contact are not on the Notes page, only on this contact's timeline.
          const linkProps = isNote
            ? { href: `#timeline-note-${String(item.source.id)}` }
            : {
                component: RouterLink,
                to: `/activities?search=${encodeURIComponent(item.source.label.replace(/…$/, ''))}`,
              };
          return (
            <Box key={item.image.id} role="listitem" sx={{ minWidth: 0 }}>
              <ButtonBase
                aria-label={t('images.openRelated', { n: index + 1, total: items.length })}
                onClick={() => {
                  onOpen(index);
                }}
                sx={{ width: '100%', aspectRatio: '1', borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
              >
                <Box
                  component="img"
                  src={thumbnailSrc(item.image.thumb_url)}
                  alt=""
                  loading="lazy"
                  sx={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                />
              </ButtonBase>
              <Typography variant="caption" color="text.secondary" display="block">
                {formatDate(item.source.date)}
              </Typography>
              <Link
                {...linkProps}
                variant="caption"
                underline="hover"
                sx={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
              >
                {item.source.label || t('contactDetail.note')}
              </Link>
            </Box>
          );
        })}
      </Box>
      {items.length < total && (
        <Button size="small" sx={{ mt: 1 }} onClick={onMore}>
          {t('images.showMore')}
        </Button>
      )}
    </Box>
  );
}
