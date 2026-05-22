import { init } from './popup';

if (document.readyState !== 'loading') {
  void init();
} else {
  document.addEventListener('DOMContentLoaded', () => void init());
}
