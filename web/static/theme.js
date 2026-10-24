'use strict';
(() => {
  const storageKey = 'icon-svg-theme';
  const root = document.documentElement;
  const buttons = () => document.querySelectorAll('#theme-toggle');

  function applyTheme(value) {
    const theme = value === 'dark' ? 'dark' : 'light';
    root.dataset.theme = theme;
    const label = theme === 'dark' ? '切换为浅色模式' : '切换为深色模式';
    buttons().forEach(button => {
      button.setAttribute('aria-label', label);
      button.setAttribute('title', label);
      button.setAttribute('aria-pressed', String(theme === 'dark'));
      button.textContent = theme === 'dark' ? '☀' : '◐';
    });
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'dark' ? '#171923' : '#f7f8fc');
  }

  let saved;
  try { saved = localStorage.getItem(storageKey); } catch {}
  // This script runs in the head, before styles and page content are painted.
  applyTheme(saved);
  document.addEventListener('DOMContentLoaded', () => {
    applyTheme(root.dataset.theme);
    buttons().forEach(button => button.addEventListener('click', () => {
      const theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
      applyTheme(theme);
      try { localStorage.setItem(storageKey, theme); } catch {}
    }));
  });
  window.addEventListener('storage', event => {
    if (event.key === storageKey || event.key === null) applyTheme(event.newValue);
  });
})();
