import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Button, Card, CardContent, Typography } from '@mui/material';
import AddPhotoAlternateIcon from '@mui/icons-material/AddPhotoAlternate';
import ImageGrid from './ImageGrid';
import FaceCropDialog from './FaceCropDialog';
import { setContactImages, setMainImage } from '../../api/contacts';
import type { ImageRef } from '../../api/images';
import { useImageInput } from '../../hooks/useImageInput';
import { handleError } from '../../utils/errorHandler';

interface ContactPhotosProps {
  contactId: number;
  images: ImageRef[];
  mainId: number | null;
  onOpen: (index: number) => void;
  onAdd: (file?: File) => void;
  onChanged: () => Promise<void>;
}

export default function ContactPhotos({ contactId, images, mainId, onOpen, onAdd, onChanged }: ContactPhotosProps) {
  const { t } = useTranslation();
  const [cropping, setCropping] = useState<ImageRef | null>(null);

  const { dropProps } = useImageInput({
    enabled: false,
    onFiles: ([file]) => {
      onAdd(file);
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

  if (images.length === 0) {
    return (
      <Box {...dropProps} sx={{ mb: 1.5 }}>
        <Button
          size="small"
          startIcon={<AddPhotoAlternateIcon />}
          onClick={() => {
            onAdd();
          }}
        >
          {t('images.addPhoto')}
        </Button>
      </Box>
    );
  }

  return (
    <Card sx={{ mb: 1.5 }} {...dropProps}>
      <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          {t('images.photos')}
        </Typography>
        <ImageGrid
          images={images}
          mainId={mainId}
          onOpen={onOpen}
          onAdd={() => {
            onAdd();
          }}
          onFrame={setCropping}
          onDelete={(image) => {
            void remove(image);
          }}
        />
      </CardContent>
      <FaceCropDialog
        image={cropping}
        onClose={() => {
          setCropping(null);
        }}
        onSave={saveCrop}
      />
    </Card>
  );
}
