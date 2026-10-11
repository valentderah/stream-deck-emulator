import * as api from './api.js';
import { createDeck } from './deck.js';
import { loadFallback, pickLang, setLang, t, translate } from './i18n.js';

const card = document.getElementById('device');
const picker = document.getElementById('picker');
const statusLamp = document.getElementById('status');
const toggle = document.getElementById('toggle');
const notice = document.getElementById('notice');
const stage = document.getElementById('stage');
const hints = document.getElementById('hints');

const devices = new Map();
// The device on the page.
let current = null;

function isLive(d) {
  return d.link.state === 'connected' || d.link.state === 'connecting';
}

function renderLink(d) {
  const { state } = d.link;
  d.option.textContent = isLive(d) ? `${d.info.name} (${t(`status.${state}`)})` : d.info.name;
  if (d === current) renderBar();
}

// renderBar shows the link of the current device in the controls above it.
function renderBar() {
  const { state, code, detail } = current.link;
  card.classList.toggle('off', state !== 'connected');
  statusLamp.dataset.state = state;
  statusLamp.title = t(`status.${state}`);
  statusLamp.setAttribute('aria-label', statusLamp.title);
  toggle.textContent = t(isLive(current) ? 'action.disconnect' : 'action.connect');
  const shown = current.actionError ?? code;
  notice.textContent = shown ? t(`error.${shown}`, { detail: detail ?? '' }) : '';
}

function renderText() {
  document.title = t('app.title');
  picker.setAttribute('aria-label', t('device.label'));
  translate(document);
  for (const d of devices.values()) renderLink(d);
}

function addDevice(info) {
  const d = { info, link: info.link, actionError: null };
  d.deck = createDeck(info, body => api.sendInput(info.id, body));
  d.deck.panel.hidden = true;
  stage.append(d.deck.panel);
  d.option = new Option('', info.id);
  picker.add(d.option);
  devices.set(info.id, d);
}

function savedDevice() {
  try { return localStorage.getItem('device'); } catch { return null; }
}

// pickDevice prefers the last choice, then a device that is already live.
function pickDevice() {
  const saved = savedDevice();
  if (devices.has(saved)) return saved;
  const live = [...devices.values()].find(isLive);
  return (live ?? devices.values().next().value)?.info.id;
}

function showDevice(id) {
  current = devices.get(id) ?? null;
  if (!current) return;
  for (const d of devices.values()) d.deck.panel.hidden = d !== current;
  picker.value = id;
  hints.replaceChildren();
  for (const key of current.deck.hints) {
    const line = document.createElement('p');
    line.className = 'hint';
    line.dataset.i18n = key;
    line.textContent = t(key);
    hints.append(line);
  }
  try { localStorage.setItem('device', id); } catch { /* private mode */ }
  renderBar();
}

// known wraps an event handler so that it gets the device instead of its id
// and is skipped for a device the page does not have.
const known = fn => (id, ...args) => {
  const d = devices.get(id);
  if (d) fn(d, ...args);
};

function listen() {
  api.listen({
    open: () => devices.forEach(d => d.deck.refreshAll()),
    key: known((d, index) => d.deck.refreshKey(index)),
    strip: known(d => d.deck.refreshStrip()),
    screen: known(d => d.deck.refreshScreen()),
    touchkey: known((d, index, rgb) => d.deck.setTouchColor(index, rgb)),
    brightness: known((d, percent) => d.deck.setBrightness(percent)),
    reset: known(d => {
      d.deck.refreshAll();
      for (let i = 0; i < d.info.touchKeys; i++) d.deck.setTouchColor(i, 0);
    }),
    link: known((d, status) => {
      d.link = status;
      d.actionError = null;
      renderLink(d);
    }),
  });
}

async function init() {
  await loadFallback();
  for (const info of await api.listDevices()) addDevice(info);
  picker.addEventListener('change', () => showDevice(picker.value));
  toggle.addEventListener('click', async () => {
    const d = current;
    if (!d) return;
    d.actionError = await (isLive(d) ? api.disconnect : api.connect)(d.info.id);
    renderLink(d);
  });
  showDevice(pickDevice());
  await setLang(pickLang());
  renderText();
  listen();
}

init();
