import { useTranslation } from 'react-i18next';
import Lightbox from 'yet-another-react-lightbox';
import Zoom from 'yet-another-react-lightbox/plugins/zoom';
import Counter from 'yet-another-react-lightbox/plugins/counter';
import Captions from 'yet-another-react-lightbox/plugins/captions';
import Download from 'yet-another-react-lightbox/plugins/download';
import 'yet-another-react-lightbox/styles.css';
import 'yet-another-react-lightbox/plugins/counter.css';
import 'yet-another-react-lightbox/plugins/captions.css';
import type { ImageRef } from '../../api/images';
import { apiUrl } from '../../utils/thumbnail';
import { useDateFormat } from '../../DateFormatProvider';

interface ImageLightboxProps {
  images: ImageRef[];
  index: number | null;
  onClose: () => void;
}

export default function ImageLightbox({ images, index, onClose }: ImageLightboxProps) {
  const { t } = useTranslation();
  const { formatDate } = useDateFormat();

  return (
    <Lightbox
      open={index !== null}
      index={index ?? 0}
      close={onClose}
      plugins={[Zoom, Counter, Captions, Download]}
      slides={images.map((image) => ({
        src: apiUrl(image.url),
        width: image.width,
        height: image.height,
        description: image.taken_at ? formatDate(image.taken_at.slice(0, 10)) : undefined,
        download: `${apiUrl(image.url)}&download=1`,
      }))}
      captions={{ descriptionTextAlign: 'center' }}
      styles={{ container: { backgroundColor: '#000' } }}
      carousel={{ finite: images.length < 2 }}
      labels={{
        Previous: t('images.lightbox.previous'),
        Next: t('images.lightbox.next'),
        Close: t('images.lightbox.close'),
        'Zoom in': t('images.lightbox.zoomIn'),
        'Zoom out': t('images.lightbox.zoomOut'),
        Download: t('images.lightbox.download'),
      }}
    />
  );
}
