
export interface RevisionEvents {
  setRange(from: number, to: number, live: boolean): void;
}

const SLOP_PX = 8;
