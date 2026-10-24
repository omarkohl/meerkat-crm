export interface Area {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface Size {
  width: number;
  height: number;
}

export interface Crop {
  x: number;
  y: number;
  size: number;
}

// Maps a crop drawn on `from` (what the browser showed) onto `to` (what the server stored, which may
// be scaled down) as a square that lies inside it.
export function toServerCrop(area: Area, from: Size, to: Size): Crop {
  const scale = to.width / from.width;
  const size = Math.max(1, Math.min(Math.round(Math.min(area.width, area.height) * scale), to.width, to.height));
  const x = Math.min(Math.max(0, Math.round(area.x * scale)), to.width - size);
  const y = Math.min(Math.max(0, Math.round(area.y * scale)), to.height - size);
  return { x, y, size };
}
