export function formatCount(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '0';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 10_000) return `${(value / 1000).toFixed(0)}k`;
  if (value >= 1000) return `${(value / 1000).toFixed(1)}k`;
  return String(Math.floor(value));
}

export function formatBadge(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '';
  if (value >= 100_000) return '99k+';
  if (value >= 10_000) return `${Math.floor(value / 1000)}k`;
  if (value >= 1000) return `${(value / 1000).toFixed(1)}k`;
  return String(Math.floor(value));
}
