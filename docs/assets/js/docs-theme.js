/* Shared GitHub Pages docs theme control; no new dependencies or network calls. */
(() => {
  const key = 'draforge-docs-theme';
  const root = document.documentElement;
  const system = window.matchMedia('(prefers-color-scheme: dark)');

  const storedPreference = () => {
    try {
      const saved = window.localStorage.getItem(key);
      return saved === 'dark' || saved === 'light' ? saved : null;
    } catch {
      return null;
    }
  };

  // This file is loaded in the document head, before the page becomes visible.
  // Apply the saved preference or the OS setting without a light-theme flash.
  root.dataset.docsTheme = storedPreference() ?? (system.matches ? 'dark' : 'light');

  const useTheme = (value) => {
    root.dataset.docsTheme = value;
    root.style.colorScheme = value;
  };

  const markScrollableCode = () => {
    document.querySelectorAll('.markdown-body pre').forEach((code) => {
      if (code.scrollWidth > code.clientWidth || code.scrollHeight > code.clientHeight) {
        code.tabIndex = 0;
        code.setAttribute('aria-label', 'Scrollable code example');
      }
    });
  };

  const addControl = () => {
    const main = document.querySelector('.markdown-body');
    if (main && !document.querySelector('main, [role="main"]')) {
      main.setAttribute('role', 'main');
    }
    markScrollableCode();
    const heading = document.querySelector('.markdown-body > h1:first-child');
    if (!heading || document.getElementById('docs-theme-toggle')) return;

    const header = document.createElement('div');
    header.className = 'docs-site-header';

    heading.parentNode.insertBefore(header, heading);
    header.appendChild(heading);

    const button = document.createElement('button');
    button.id = 'docs-theme-toggle';
    button.type = 'button';
    button.className = 'docs-theme-toggle';

    const updateButton = () => {
      const dark = root.dataset.docsTheme === 'dark';
      button.textContent = dark ? '☀ Light mode' : '☾ Dark mode';
      button.setAttribute('aria-label', dark ? 'Switch to light mode' : 'Switch to dark mode');
      button.setAttribute('aria-pressed', String(dark));
      button.title = dark ? 'Use light colors' : 'Use dark colors';
    };

    button.addEventListener('click', () => {
      const next = root.dataset.docsTheme === 'dark' ? 'light' : 'dark';
      useTheme(next);
      try {
        window.localStorage.setItem(key, next);
      } catch {
        // The current tab still switches when storage is unavailable.
      }
      updateButton();
    });

    header.appendChild(button);
    updateButton();
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', addControl, { once: true });
  } else {
    addControl();
  }

  const followSystem = () => {
    if (!storedPreference()) {
      useTheme(system.matches ? 'dark' : 'light');
      const control = document.getElementById('docs-theme-toggle');
      if (control) {
        const dark = system.matches;
        control.textContent = dark ? '☀ Light mode' : '☾ Dark mode';
        control.setAttribute('aria-label', dark ? 'Switch to light mode' : 'Switch to dark mode');
        control.setAttribute('aria-pressed', String(dark));
      }
    }
  };

  if (typeof system.addEventListener === 'function') {
    system.addEventListener('change', followSystem);
  }
  window.addEventListener('resize', markScrollableCode, { passive: true });
})();
