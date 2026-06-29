/** @type {import('jest').Config} */
// ESM mode: the action is now an ES module (package.json "type":"module") so it
// can consume the ESM-only @actions/core v3 / @actions/glob v0.7. ts-jest runs
// under Node's experimental VM modules (see the test script's NODE_OPTIONS).
module.exports = {
  preset: 'ts-jest/presets/default-esm',
  testEnvironment: 'node',
  extensionsToTreatAsEsm: ['.ts'],
  roots: ['<rootDir>/src', '<rootDir>/__tests__'],
  testMatch: ['**/__tests__/**/*.test.ts'],
  moduleNameMapper: {
    // ESM source uses explicit .js specifiers; map them back to the .ts sources.
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
  transform: {
    '^.+\\.ts$': ['ts-jest', { useESM: true }],
  },
  collectCoverageFrom: ['src/**/*.ts', '!src/main.ts'],
  coverageDirectory: 'coverage',
  coverageReporters: ['text', 'lcov', 'html', 'json-summary'],
  coverageThreshold: {
    global: {
      branches: 90,
      functions: 90,
      lines: 90,
      statements: 90,
    },
  },
  verbose: true,
  clearMocks: true,
};
