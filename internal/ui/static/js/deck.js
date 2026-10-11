import { keyUrl, stripUrl, screenUrl } from './api.js';

const LONG_PRESS_MS = 500;
const DRAG_PX = 20;
const TOUCH_SHORT = 1, TOUCH_LONG = 2, TOUCH_DRAG = 3;
const DIAL_STEP_DEG = 12;
// A real panel at 0% brightness is dim, not black.
const MIN_BRIGHTNESS = 0.25;
const RGB_HEX_DIGITS = 6;

function el(tag, className, parent) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  parent?.append(node);
  return node;
}

// picture makes an image that keeps its old picture on screen until the next
// one has loaded, and goes blank when the device has none.
function picture(className, parent) {
  const img = el('img', className, parent);
  img.alt = '';
  img.draggable = false;
  img.addEventListener('error', () => img.removeAttribute('src'));
  return img;
}

// pressable reports presses of node, releasing it when the pointer leaves.
function pressable(node, report) {
  let down = false;
  const set = value => {
    if (down === value) return;
    down = value;
    node.classList.toggle('down', value);
    report(value);
  };
  node.addEventListener('pointerdown', () => set(true));
  node.addEventListener('pointerup', () => set(false));
  node.addEventListener('pointerleave', () => set(false));
}

function buildKeys(deck, info, send) {
  for (let i = 0; i < info.rows * info.cols; i++) {
    const key = el('div', 'key', deck.panel);
    deck.keys.push(picture(info.rotated ? 'face rotated' : 'face', key));
    pressable(key, down => send({ type: 'key', index: i, down }));
  }
}

function buildStrip(deck, info, send) {
  const { w, h } = info.strip;
  const strip = deck.strip = picture('strip', deck.panel);
  strip.style.setProperty('--w', w);
  strip.style.setProperty('--h', h);
  const point = e => {
    const r = strip.getBoundingClientRect();
    const clamp = (v, max) => Math.max(0, Math.min(max - 1, Math.round(v)));
    return { x: clamp((e.clientX - r.left) * w / r.width, w), y: clamp((e.clientY - r.top) * h / r.height, h) };
  };
  let start = null;
  strip.addEventListener('pointerdown', e => {
    strip.setPointerCapture(e.pointerId);
    start = { ...point(e), time: performance.now() };
  });
  strip.addEventListener('pointerup', e => {
    if (!start) return;
    const from = start, to = point(e);
    start = null;
    if (Math.hypot(to.x - from.x, to.y - from.y) > DRAG_PX) {
      send({ type: 'touch', kind: TOUCH_DRAG, x: from.x, y: from.y, x2: to.x, y2: to.y });
    } else {
      const kind = performance.now() - from.time >= LONG_PRESS_MS ? TOUCH_LONG : TOUCH_SHORT;
      send({ type: 'touch', kind, x: from.x, y: from.y });
    }
  });
  strip.addEventListener('pointercancel', () => { start = null; });
}

function buildDials(deck, info, send) {
  for (let i = 0; i < info.dials; i++) {
    const dial = el('div', 'dial', deck.panel);
    let angle = 0;
    dial.addEventListener('wheel', e => {
      e.preventDefault();
      const ticks = e.deltaY < 0 ? 1 : -1;
      angle += ticks * DIAL_STEP_DEG;
      dial.style.rotate = `${angle}deg`;
      send({ type: 'dialRotate', index: i, ticks });
    }, { passive: false });
    pressable(dial, down => send({ type: 'dialPress', index: i, down }));
  }
}

// buildInfoBar lays out the touch keys around the info screen.
function buildInfoBar(deck, info, send) {
  const keyCount = info.rows * info.cols;
  const touchKey = i => {
    const node = deck.touchKeys[i] = el('div', 'touchkey', deck.panel);
    pressable(node, down => send({ type: 'key', index: keyCount + i, down }));
  };
  const left = Math.ceil(info.touchKeys / 2);
  for (let i = 0; i < left; i++) touchKey(i);
  if (info.screen) {
    const screen = el('div', 'screen', deck.panel);
    screen.style.setProperty('--w', info.screen.w);
    screen.style.setProperty('--h', info.screen.h);
    screen.style.setProperty('--span', Math.max(1, info.cols - info.touchKeys));
    deck.screen = picture(info.rotated ? 'face rotated' : 'face', screen);
  }
  for (let i = left; i < info.touchKeys; i++) touchKey(i);
}

// createDeck builds the panel of one model. send(body) delivers its input.
export function createDeck(info, send) {
  const deck = { panel: el('div', 'panel'), keys: [], touchKeys: [], strip: null, screen: null };
  deck.panel.dataset.model = info.id;
  deck.panel.style.setProperty('--cols', info.cols);
  buildKeys(deck, info, send);
  if (info.strip) buildStrip(deck, info, send);
  if (info.dials) buildDials(deck, info, send);
  if (info.screen || info.touchKeys) buildInfoBar(deck, info, send);

  const hints = [];
  if (info.dials) hints.push('hint.dial');
  if (info.strip) hints.push('hint.strip');
  if (info.touchKeys) hints.push('hint.touchkey');

  const refreshKey = i => { deck.keys[i].src = keyUrl(info.id, i); };
  const refreshStrip = () => { if (deck.strip) deck.strip.src = stripUrl(info.id); };
  const refreshScreen = () => { if (deck.screen) deck.screen.src = screenUrl(info.id); };

  return {
    panel: deck.panel,
    hints,
    refreshKey,
    refreshStrip,
    refreshScreen,
    refreshAll() {
      deck.keys.forEach((_, i) => refreshKey(i));
      refreshStrip();
      refreshScreen();
    },
    setTouchColor(i, rgb) {
      const node = deck.touchKeys[i];
      if (!node) return;
      if (rgb) node.style.setProperty('--color', `#${rgb.toString(16).padStart(RGB_HEX_DIGITS, '0')}`);
      else node.style.removeProperty('--color');
    },
    setBrightness(percent) {
      deck.panel.style.setProperty('--brightness', MIN_BRIGHTNESS + (1 - MIN_BRIGHTNESS) * percent / 100);
    },
  };
}
