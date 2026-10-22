import { thumbnailSrc } from '../../utils/thumbnail';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { Box, Button, ButtonBase, Card, CardContent, Link, Typography } from '@mui/material';
import { getRelatedImages, type RelatedImage } from '../../api/images';
import { useDateFormat } from '../../DateFormatProvider';
import { handleError } from '../../utils/errorHandler';
import ImageLightbox from './ImageLightbox';

interface RelatedImagesProps {
  contactId: number;
  // Changes when the notes or activities of the contact do, so the list is read again.
  version: string;
}

const PAGE = 12;
const TILE = 96;

export default function RelatedImages({ contactId, version }: RelatedImagesProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();
  const [items, setItems] = useState<RelatedImage[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [lightbox, setLightbox] = useState<number | null>(null);

  useEffect(() => {
    let current = true;
    getRelatedImages(contactId, 1, PAGE)
      .then((first) => {
        if (!current) return;
        setItems(first.images);
        setTotal(first.total);
        setPage(1);
      })
      .catch((err: unknown) => {
        handleError(err, { operation: 'loading related images' });
      });
    return () => {
      current = false;
    };
  }, [contactId, version]);

  const more = async () => {
    try {
      const next = await getRelatedImages(contactId, page + 1, PAGE);
      setItems((prev) => [...prev, ...next.images]);
      setTotal(next.total);
      setPage(page + 1);
    } catch (err) {
      handleError(err, { operation: 'loading related images' });
    }
  };

  if (total === 0) return null;

  return (
    <Card sx={{ mb: 1.5 }}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          {t('images.related')}
        </Typography>
        <Box role="list" sx={{ display: 'flex', flexWrap: 'wrap', gap: 1.5 }}>
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
              <Box key={item.image.id} role="listitem" sx={{ width: TILE }}>
                <ButtonBase
                  aria-label={t('images.openRelated', { n: index + 1, total: items.length })}
                  onClick={() => {
                    setLightbox(index);
                  }}
                  sx={{ width: TILE, height: TILE, borderRadius: 1, overflow: 'hidden', bgcolor: 'action.hover' }}
                >
                  <Box
                    component="img"
                    src={thumbnailSrc(item.image.thumb_url)}
                    alt=""
                    loading="lazy"
                    sx={{ width: TILE, height: TILE, objectFit: 'cover', display: 'block' }}
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
          <Button
            size="small"
            sx={{ mt: 1 }}
            onClick={() => {
              void more();
            }}
          >
            {t('images.showMore')}
          </Button>
        )}
      </CardContent>
      <ImageLightbox
        images={items.map((i) => i.image)}
        index={lightbox}
        onClose={() => {
          setLightbox(null);
        }}
      />
    </Card>
  );
}
