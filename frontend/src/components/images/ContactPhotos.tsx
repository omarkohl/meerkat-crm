import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Button, Divider, Stack, Typography } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import ImageGrid from './ImageGrid';
import FaceCropDialog from './FaceCropDialog';
import ImageLightbox from './ImageLightbox';
import RelatedImages from './RelatedImages';
import { setContactImages, setMainImage } from '../../api/contacts';
import type { ImageRef } from '../../api/images';
import { useImageInput } from '../../hooks/useImageInput';
import type { useRelatedImages } from '../../hooks/useRelatedImages';
import { handleError } from '../../utils/errorHandler';

interface ContactPhotosProps {
  contactId: number;
  images: ImageRef[];
  mainId: number | null;
  related: ReturnType<typeof useRelatedImages>;
  onAdd: (files?: File[]) => void;
  onChanged: () => Promise<void>;
}

export default function ContactPhotos({ contactId, images, mainId, related, onAdd, onChanged }: ContactPhotosProps) {
  const { t } = useTranslation();
  const [cropping, setCropping] = useState<ImageRef | null>(null);
  const [lightbox, setLightbox] = useState<number | null>(null);

  const { dropProps } = useImageInput({
    enabled: false,
    onFiles: (files) => {
      onAdd(files);
    },
  });

  const saveCrop = async (image: ImageRef, crop: { x: number; y: number; size: number }) => {
    await setMainImage(contactId, image.id, crop);
    await onChanged();
  };

  const remove = async (image: ImageRef) => {
    try {
      await setContactImages(
        contactId,
        images.filter((i) => i.id !== image.id).map((i) => i.id),
      );
      await onChanged();
    } catch (err) {
      handleError(err, { operation: 'deleting contact image' });
    }
  };

  const slides = [...images, ...related.items.map((i) => i.image)];

  // Nearing the end of the loaded photos fetches the next page, so the gallery has no dead end.
  const onView = (index: number) => {
    if (related.items.length < related.total && index >= slides.length - 2) void related.more();
  };

  return (
    <Box {...dropProps} sx={{ p: 2 }}>
      <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1.5 }}>
        <Button
          startIcon={<AddPhotoAlternateIcon />}
          variant="outlined"
          size="small"
          onClick={() => {
            onAdd();
          }}
        >
          {t('images.addPhoto')}
        </Button>
      </Box>
      <Stack spacing={2} divider={<Divider />}>
        {slides.length === 0 && (
          <Typography variant="body2" color="text.secondary">
            {t('images.noPhotos')}
          </Typography>
        )}
        {images.length > 0 && (
          <Box>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              {t('images.contactPhotos')}
            </Typography>
            <ImageGrid
              images={images}
              mainId={mainId}
              onOpen={setLightbox}
              onFrame={setCropping}
              onDelete={(image) => {
                void remove(image);
              }}
            />
          </Box>
        )}
        {related.items.length > 0 && (
          <RelatedImages
            items={related.items}
            total={related.total}
            onOpen={(index) => {
              setLightbox(images.length + index);
            }}
            onMore={() => {
              void related.more();
            }}
          />
        )}
      </Stack>
      <FaceCropDialog
        image={cropping}
        onClose={() => {
          setCropping(null);
        }}
        onSave={saveCrop}
      />
      <ImageLightbox
        images={slides}
        index={lightbox}
        onClose={() => {
          setLightbox(null);
        }}
        onView={onView}
      />
    </Box>
  );
}
