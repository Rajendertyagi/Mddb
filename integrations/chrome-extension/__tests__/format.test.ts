import { formatBadge, formatCount } from '../src/format';

describe('formatCount', () => {
  it.each([
    [0, '0'],
    [42, '42'],
    [999, '999'],
    [1000, '1.0k'],
    [1234, '1.2k'],
    [9999, '10.0k'],
    [10_000, '10k'],
    [123_456, '123k'],
    [1_500_000, '1.5M'],
  ])('formats %d as %s', (input, expected) => {
    expect(formatCount(input)).toBe(expected);
  });

  it('handles invalid values', () => {
    expect(formatCount(-5)).toBe('0');
    expect(formatCount(Number.NaN)).toBe('0');
  });
});

describe('formatBadge', () => {
  it.each([
    [0, '0'],
    [42, '42'],
    [999, '999'],
    [1234, '1.2k'],
    [10_000, '10k'],
    [50_000, '50k'],
    [100_000, '99k+'],
    [9_999_999, '99k+'],
  ])('formats %d as %s', (input, expected) => {
    expect(formatBadge(input)).toBe(expected);
  });

  it('returns empty string for invalid values', () => {
    expect(formatBadge(-1)).toBe('');
    expect(formatBadge(Number.NaN)).toBe('');
  });
});
