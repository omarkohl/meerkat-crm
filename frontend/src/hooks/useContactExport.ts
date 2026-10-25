import { exportContactAsVcf } from '../api/export';

export function useContactExport() {
  return { exportVcard: exportContactAsVcf };
}
