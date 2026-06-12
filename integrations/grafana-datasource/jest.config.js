/** Jest config — pure-logic coverage gate (≥90%) on query/transform/client/datasource. */
//
// `@grafana/data` (and its CJS `dist/cjs/index.cjs`) transitively requires a
// handful of ESM-only d3 modules. Jest skips `node_modules` for transforms by
// default, so we override that for the specific ESM packages Grafana pulls in,
// otherwise `import` statements blow up with SyntaxError. We also map .mjs to
// the ts-jest transform.
const ESM_NODE_MODULES = [
  'd3',
  'd3-array',
  'd3-color',
  'd3-format',
  'd3-interpolate',
  'd3-path',
  'd3-scale',
  'd3-shape',
  'd3-time',
  'd3-time-format',
  'd3-timer',
  'internmap',
  'delaunator',
  'robust-predicates',
  'ol',
  'rxjs',
  '@grafana',
  'react-colorful',
  'uuid',
  'nanoid',
  'memoize-one',
].join('|');

module.exports = {
  preset: 'ts-jest',
  // jsdom — @grafana/data touches `window` even from its data-frame helpers.
  testEnvironment: 'jsdom',
  setupFiles: ['<rootDir>/__tests__/setup.ts'],
  rootDir: '.',
  roots: ['<rootDir>/__tests__'],
  testMatch: ['**/__tests__/**/*.test.ts'],
  moduleFileExtensions: ['ts', 'tsx', 'js', 'jsx', 'mjs', 'json'],
  transform: {
    '^.+\\.(ts|tsx|mjs|js|jsx)$': ['ts-jest', { useESM: false }],
  },
  transformIgnorePatterns: [`/node_modules/(?!(${ESM_NODE_MODULES})/)`],
  moduleNameMapper: {
    // @grafana/ui transitively imports CSS from rc-picker / @emotion / etc.
    // Map those to an empty stub so Jest stops treating them as JavaScript.
    '\\.(css|less|scss|sass)$': '<rootDir>/__tests__/cssStub.ts',
    '\\.(png|jpg|jpeg|gif|svg|woff|woff2)$': '<rootDir>/__tests__/cssStub.ts',
    // @grafana/runtime drags in @grafana/ui → uplot → matchMedia. Tests
    // inject their own fetcher/interpolator, so the real runtime is dead
    // weight — replace it with a minimal stub.
    '^@grafana/runtime$': '<rootDir>/__tests__/grafanaRuntimeStub.ts',
  },
  collectCoverageFrom: [
    'src/client.ts',
    'src/datasource.ts',
    'src/query.ts',
    'src/transform.ts',
  ],
  coverageDirectory: 'coverage',
  coverageReporters: ['text', 'lcov', 'json-summary'],
  coverageThreshold: {
    global: {
      statements: 90,
      branches: 90,
      functions: 90,
      lines: 90,
    },
  },
};
