import { derivePanelUrl, normalizeServerUrl, originOf } from '../src/url';

describe('normalizeServerUrl', () => {
  it.each([
    ['https://mddb.example.com', 'https://mddb.example.com'],
    ['https://mddb.example.com/', 'https://mddb.example.com'],
    ['https://mddb.example.com/api///', 'https://mddb.example.com/api'],
    ['http://localhost:11023', 'http://localhost:11023'],
    ['  https://mddb.example.com  ', 'https://mddb.example.com'],
  ])('normalizes %s', (input, expected) => {
    expect(normalizeServerUrl(input)).toBe(expected);
  });

  it('rejects empty input', () => {
    expect(() => normalizeServerUrl('   ')).toThrow(/required/);
  });

  it('rejects non-URL input', () => {
    expect(() => normalizeServerUrl('not a url')).toThrow(/valid absolute URL/);
  });

  it('rejects unsupported protocols', () => {
    expect(() => normalizeServerUrl('ftp://example.com')).toThrow(/http/);
  });
});

describe('derivePanelUrl', () => {
  it('uses override when provided', () => {
    expect(derivePanelUrl('https://mddb.example.com', 'https://panel.example.com')).toBe(
      'https://panel.example.com',
    );
  });

  it('falls back to server origin with port 3000', () => {
    expect(derivePanelUrl('https://mddb.example.com')).toBe('https://mddb.example.com:3000');
  });

  it('handles localhost', () => {
    expect(derivePanelUrl('http://localhost:11023')).toBe('http://localhost:3000');
  });

  it('ignores blank override', () => {
    expect(derivePanelUrl('https://mddb.example.com', '   ')).toBe('https://mddb.example.com:3000');
  });
});

describe('originOf', () => {
  it('returns the origin component', () => {
    expect(originOf('https://mddb.example.com/api/v1/foo?q=bar')).toBe('https://mddb.example.com');
  });
});
