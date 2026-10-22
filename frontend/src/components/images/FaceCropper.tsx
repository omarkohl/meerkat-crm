import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import Cropper from 'react-easy-crop';
import { Box, Slider } from '@mui/material';
import ZoomInIcon from '@mui/icons-material/ZoomIn';
import type { Area, Size } from '../../utils/faceCrop';

interface FaceCropperProps {
  src: string;
  initialArea?: Area;
  onChange: (area: Area, natural: Size) => void;
}

export default function FaceCropper({ src, initialArea, onChange }: FaceCropperProps) {
  const { t } = useTranslation();
  const [crop, setCrop] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [natural, setNatural] = useState<Size | null>(null);

  const onCropComplete = useCallback(
    (_percent: Area, pixels: Area) => {
      if (natural) onChange(pixels, natural);
    },
    [natural, onChange],
  );

  return (
    <Box>
      <Box
        sx={{ position: 'relative', width: '100%', height: 350, bgcolor: 'black', borderRadius: 1, overflow: 'hidden' }}
      >
        <Cropper
          image={src}
          crop={crop}
          zoom={zoom}
          aspect={1}
          onCropChange={setCrop}
          onZoomChange={setZoom}
          onMediaLoaded={(media) => {
            setNatural({ width: media.naturalWidth, height: media.naturalHeight });
          }}
          onCropComplete={onCropComplete}
          initialCroppedAreaPixels={initialArea}
          cropShape="round"
          showGrid={false}
        />
      </Box>
      <Box sx={{ display: 'flex', alignItems: 'center', mt: 2, px: 2 }}>
        <ZoomInIcon sx={{ mr: 2, color: 'text.secondary' }} />
        <Slider
          value={zoom}
          min={1}
          max={3}
          step={0.1}
          onChange={(_, value) => {
            setZoom(value);
          }}
          aria-label={t('profilePicture.zoom')}
        />
      </Box>
    </Box>
  );
}
