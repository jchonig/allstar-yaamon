'use strict';

const THEMES = ['dark', 'light', 'solarized', 'high-contrast'];

function applyTheme(theme) {
  document.documentElement.setAttribute('data-theme', theme);
  localStorage.setItem('yaamon_theme', theme);
}

function cycleTheme() {
  const current = localStorage.getItem('yaamon_theme') || 'dark';
  const next = THEMES[(THEMES.indexOf(current) + 1) % THEMES.length];
  applyTheme(next);
}

document.addEventListener('DOMContentLoaded', function () {
  const btn = document.getElementById('themeToggle');
  if (btn) btn.addEventListener('click', cycleTheme);
});
