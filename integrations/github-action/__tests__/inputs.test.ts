import {
  assertKeyPrefix,
  assertKeyStrategy,
  normaliseUrl,
  parseBool,
  parseInteger,
  readInputs,
  splitPatterns,
} from '../src/inputs.js';

describe('parseBool', () => {
  it('returns the fallback for an empty string', () => {
    expect(parseBool('', true)).toBe(true);
    expect(parseBool('   ', false)).toBe(false);
  });

  it('accepts common truthy and falsy spellings', () => {
    for (const v of ['true', 'TRUE', '1', 'yes', 'on']) expect(parseBool(v, false)).toBe(true);
    for (const v of ['false', 'FALSE', '0', 'no', 'off']) expect(parseBool(v, true)).toBe(false);
  });

  it('throws for nonsense values', () => {
    expect(() => parseBool('maybe', false)).toThrow(/Invalid boolean/);
  });
});

describe('parseInteger', () => {
  it('parses integers within range', () => {
    expect(parseInteger('5', 'concurrency', 1, 64)).toBe(5);
  });

  it('rejects non-numeric values', () => {
    expect(() => parseInteger('abc', 'concurrency', 1, 64)).toThrow(/Must be an integer/);
  });

  it('rejects out-of-range values', () => {
    expect(() => parseInteger('0', 'concurrency', 1, 64)).toThrow(/Must be between/);
    expect(() => parseInteger('100', 'concurrency', 1, 64)).toThrow(/Must be between/);
  });
});

describe('splitPatterns', () => {
  it('splits newline-separated patterns and strips comments / blanks', () => {
    const raw = '**/*.md\n\n# comment\n  docs/**/*.mdx  \n';
    expect(splitPatterns(raw)).toEqual(['**/*.md', 'docs/**/*.mdx']);
  });

  it('handles CR+LF line endings', () => {
    expect(splitPatterns('a.md\r\nb.md')).toEqual(['a.md', 'b.md']);
  });
});

describe('assertKeyStrategy', () => {
  it.each(['path', 'hash', 'filename', 'PATH'])('accepts %s', (v) => {
    expect(assertKeyStrategy(v)).toBe(v.toLowerCase());
  });

  it('rejects unknown strategies', () => {
    expect(() => assertKeyStrategy('uuid')).toThrow(/Invalid key-strategy/);
  });
});

describe('assertKeyPrefix', () => {
  it('accepts an empty prefix and key-safe characters', () => {
    for (const v of ['', 'site/', 'docs.v2-', 'a_b/c.d', 'A1/B2_3', '/']) {
      expect(assertKeyPrefix(v)).toBe(v);
    }
  });

  it('accepts a prefix of exactly 100 characters', () => {
    const v = 'a'.repeat(100);
    expect(assertKeyPrefix(v)).toBe(v);
  });

  it('rejects disallowed characters', () => {
    for (const v of ['site:', 'a b', 'emoji😀', 'has\tcontrol', 'with|pipe', 'q?x']) {
      expect(() => assertKeyPrefix(v)).toThrow(/Invalid key-prefix/);
    }
  });

  it('rejects a prefix longer than 100 characters', () => {
    expect(() => assertKeyPrefix('a'.repeat(101))).toThrow(/Invalid key-prefix/);
  });
});

describe('normaliseUrl', () => {
  it('strips trailing slashes', () => {
    expect(normaliseUrl('https://mddb.tradik.com///')).toBe('https://mddb.tradik.com');
  });

  it('rejects non-http(s) URLs', () => {
    expect(() => normaliseUrl('ftp://example.com')).toThrow(/Must start with http/);
  });
});

describe('readInputs', () => {
  function stubInputs(values: Record<string, string>): (name: string) => string {
    return (name: string) => values[name] ?? '';
  }

  it('parses a fully specified input set', () => {
    const inputs = readInputs(
      stubInputs({
        'mddb-url': 'https://mddb.example.com/',
        'api-key': 'vk_test',
        collection: 'docs',
        path: '**/*.md\n!docs/draft/**',
        ignore: 'node_modules/**',
        'working-directory': './subdir',
        language: 'pl_PL',
        'key-strategy': 'filename',
        'key-prefix': 'site/',
        concurrency: '4',
        'timeout-seconds': '15',
        'verify-ssl': 'false',
        'dry-run': 'true',
        'fail-on-error': 'false',
      }),
    );
    expect(inputs).toEqual({
      mddbUrl: 'https://mddb.example.com',
      apiKey: 'vk_test',
      collection: 'docs',
      patterns: ['**/*.md', '!docs/draft/**'],
      ignore: ['node_modules/**'],
      workingDirectory: './subdir',
      language: 'pl_PL',
      keyStrategy: 'filename',
      keyPrefix: 'site/',
      concurrency: 4,
      timeoutSeconds: 15,
      verifySsl: false,
      dryRun: true,
      failOnError: false,
    });
  });

  it('applies defaults when optional inputs are empty', () => {
    const inputs = readInputs(stubInputs({ collection: 'docs' }));
    expect(inputs.mddbUrl).toBe('https://mddb.tradik.com');
    expect(inputs.patterns).toEqual(['**/*.md']);
    expect(inputs.language).toBe('en_US');
    expect(inputs.keyStrategy).toBe('path');
    expect(inputs.concurrency).toBe(8);
    expect(inputs.verifySsl).toBe(true);
    expect(inputs.dryRun).toBe(false);
    expect(inputs.failOnError).toBe(true);
  });

  it('rejects missing collection', () => {
    expect(() => readInputs(stubInputs({}))).toThrow(/collection.*required/);
  });

  it('rejects empty path patterns after trimming', () => {
    expect(() =>
      readInputs(stubInputs({ collection: 'docs', path: '\n  \n# only comments\n' })),
    ).toThrow(/no patterns/);
  });
});
