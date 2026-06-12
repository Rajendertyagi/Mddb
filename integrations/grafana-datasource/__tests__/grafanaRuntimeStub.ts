/**
 * Minimal stub for `@grafana/runtime` used only in Jest. The real module
 * transitively imports `@grafana/ui` → `uplot` → `matchMedia`, which jsdom
 * doesn't ship — and we never exercise those paths from unit tests because
 * the DataSource always receives an injected `fetcher` + `templateInterpolate`.
 */
export const getBackendSrv = () => ({
  fetch: () => ({
    subscribe: () => ({ unsubscribe: () => undefined }),
  }),
});

export const getTemplateSrv = () => ({
  replace: (raw: string) => raw,
});
