'use strict';

function resolveTheme(choice) {
  if (choice === 'system') {
    return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  return choice;
}

function applyThemeChoice(choice) {
  document.documentElement.setAttribute('data-theme', resolveTheme(choice));
  localStorage.setItem('yaamon_theme', choice);
  updateThemeMenu(choice);
}

function updateThemeMenu(choice) {
  document.querySelectorAll('[data-theme-choice]').forEach(el => {
    el.classList.toggle('active', el.dataset.themeChoice === choice);
  });
}

document.addEventListener('DOMContentLoaded', function () {
  const choice = localStorage.getItem('yaamon_theme') || 'dark';
  updateThemeMenu(choice);

  document.querySelectorAll('[data-theme-choice]').forEach(el => {
    el.addEventListener('click', () => applyThemeChoice(el.dataset.themeChoice));
  });

  // Re-apply when OS theme changes and the user has chosen System.
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if ((localStorage.getItem('yaamon_theme') || 'dark') === 'system') {
      document.documentElement.setAttribute('data-theme', resolveTheme('system'));
    }
  });
});
