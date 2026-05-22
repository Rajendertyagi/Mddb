import { init } from './options';

if (document.readyState !== 'loading') {
  void init();
} else {
  document.addEventListener('DOMContentLoaded', () => void init());
}
