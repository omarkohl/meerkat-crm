import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Button, Typography } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import ImageGrid from './ImageGrid';
import FaceCropDialog from './FaceCropDialog';
import ImageLightbox from './ImageLightbox';
import { setContactImages, setMainImage } from '../../api/contacts';
import type { ImageRef } from '../../api/images';
import { useImageInput } from '../../hooks/useImageInput';
import { handleError } from '../../utils/errorHandler';

interface ContactPhotosProps {
  contactId: number;
  images: ImageRef[];
  mainId: number | null;
  onAdd: (files?: File[]) => void;
  onChanged: () => Promise<void>;
}

export default function ContactPhotos({ contactId, images, mainId, onAdd, onChanged }: ContactPhotosProps) {
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
      {images.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {t('images.noPhotos')}
        </Typography>
      ) : (
        <ImageGrid
          images={images}
          mainId={mainId}
          onOpen={setLightbox}
          onFrame={setCropping}
          onDelete={(image) => {
            void remove(image);
          }}
        />
      )}
      <FaceCropDialog
        image={cropping}
        onClose={() => {
          setCropping(null);
        }}
        onSave={saveCrop}
      />
      <ImageLightbox
        images={images}
        index={lightbox}
        onClose={() => {
          setLightbox(null);
        }}
      />
    </Box>
  );
}
