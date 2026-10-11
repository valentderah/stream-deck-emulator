const LANGS = ['en', 'ru'];

let catalog = {};
let fallback = {};

async function loadCatalog(lang) {
  const res = await fetch(`/locales/${lang}.json`);
  if (!res.ok) throw new Error(`locale ${lang}: ${res.status}`);
  return res.json();
}

export const t = (key, params = {}) =>
  (catalog[key] ?? fallback[key] ?? key).replace(/\{(\w+)\}/g, (_, name) => params[name] ?? '');

export async function loadFallback() {
  fallback = catalog = await loadCatalog('en');
}

export function pickLang() {
  for (const tag of navigator.languages ?? []) {
    const lang = tag.slice(0, 2).toLowerCase();
    if (LANGS.includes(lang)) return lang;
  }
  return 'en';
}

// setLang returns the language in use: English if lang cannot be loaded.
export async function setLang(lang) {
  try {
    catalog = lang === 'en' ? fallback : await loadCatalog(lang);
  } catch {
    lang = 'en';
    catalog = fallback;
  }
  document.documentElement.lang = lang;
  return lang;
}

// translate fills every [data-i18n] node under root.
export function translate(root) {
  for (const node of root.querySelectorAll('[data-i18n]')) node.textContent = t(node.dataset.i18n);
}
