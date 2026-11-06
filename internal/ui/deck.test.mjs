import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createDeck } from './static/js/deck.js';

class Element {
  children = [];
  dataset = {};
  style = { setProperty() {} };
  handlers = {};
  classList = { toggle() {} };
  append(child) { this.children.push(child); }
  addEventListener(type, handler) { this.handlers[type] = handler; }
}

globalThis.document = { createElement: () => new Element() };

test('key orientation corrections preserve layout and input indices', () => {
  const models = [
    ['plus', 2, 4, false, false, 'face'],
    ['neo', 2, 4, true, false, 'face rotated'],
    ['mk2', 3, 5, true, false, 'face rotated'],
    ['xl', 4, 8, true, false, 'face rotated'],
    ['plus-xl', 4, 9, false, true, 'face quarter-turn'],
    ['module', 3, 5, true, false, 'face rotated'],
    ['module-vert', 5, 3, true, false, 'face rotated'],
    ['studio', 2, 16, false, false, 'face'],
    ['pedal', 1, 3, false, false, 'face'],
  ];
  for (const [id, rows, cols, rotated, quarterTurn, className] of models) {
    const sent = [];
    const deck = createDeck({ id, rows, cols, rotated, quarterTurn }, body => sent.push(body));
    assert.equal(deck.panel.children.length, rows * cols, id);
    deck.panel.children.forEach((key, index) => {
      assert.equal(key.children[0].className, className, `${id} key ${index}`);
      key.handlers.pointerdown();
      key.handlers.pointerup();
      assert.deepEqual(sent.slice(-2), [
        { type: 'key', index, down: true },
        { type: 'key', index, down: false },
      ]);
    });
  }
});

test('Neo info screen retains its half-turn correction', () => {
  const deck = createDeck({
    id: 'neo', rows: 2, cols: 4, rotated: true, quarterTurn: false,
    touchKeys: 2, screen: { w: 248, h: 58 },
  }, () => {});
  assert.equal(deck.panel.children[9].children[0].className, 'face rotated');
});

test('+ XL strip stays horizontal with unchanged touch coordinates', () => {
  const sent = [];
  const deck = createDeck({
    id: 'plus-xl', rows: 4, cols: 9, rotated: false, quarterTurn: true,
    strip: { w: 1200, h: 100 },
  }, body => sent.push(body));
  const strip = deck.panel.children[36];
  assert.equal(strip.className, 'strip');
  strip.setPointerCapture = () => {};
  strip.getBoundingClientRect = () => ({ left: 10, top: 20, width: 600, height: 50 });
  const event = { pointerId: 1, clientX: 310, clientY: 45 };
  strip.handlers.pointerdown(event);
  strip.handlers.pointerup(event);
  assert.deepEqual(sent, [{ type: 'touch', kind: 1, x: 600, y: 50 }]);
});

test('Studio places two working dials beside the unchanged 16-by-2 key grid', () => {
  const sent = [];
  const deck = createDeck({
    id: 'studio', rows: 2, cols: 16, dials: 2,
  }, body => sent.push(body));
  assert.equal(deck.panel.children.length, 34);
  assert.deepEqual(deck.hints, ['hint.dial']);
  const keys = deck.panel.children.slice(0, 32);
  keys.forEach((key, index) => {
    assert.equal(key.style.gridColumnStart, index % 16 + 2);
    assert.equal(key.style.gridRowStart, Math.floor(index / 16) + 1);
    key.handlers.pointerdown();
    key.handlers.pointerup();
    assert.deepEqual(sent.slice(-2), [
      { type: 'key', index, down: true },
      { type: 'key', index, down: false },
    ]);
  });
  deck.panel.children.slice(32).forEach((dial, index) => {
    assert.equal(dial.className, 'dial');
    assert.equal(dial.style.gridColumnStart, index === 0 ? 1 : 18);
    assert.equal(dial.style.gridRow, '1 / span 2');
    dial.handlers.pointerdown();
    dial.handlers.pointerup();
    for (const deltaY of [-1, 1]) {
      let prevented = false;
      dial.handlers.wheel({ deltaY, preventDefault() { prevented = true; } });
      assert.ok(prevented);
    }
    assert.deepEqual(sent.slice(-4), [
      { type: 'dialPress', index, down: true },
      { type: 'dialPress', index, down: false },
      { type: 'dialRotate', index, ticks: 1 },
      { type: 'dialRotate', index, ticks: -1 },
    ]);
    assert.equal(dial.style.rotate, '0deg');
  });
});

test('Plus models keep their dials in a centered row beneath the strip', () => {
  for (const [id, rows, cols, dials, width] of [
    ['plus', 2, 4, 4, 800],
    ['plus-xl', 4, 9, 6, 1200],
  ]) {
    const deck = createDeck({
      id, rows, cols, dials, strip: { w: width, h: 100 },
    }, () => {});
    assert.equal(deck.panel.children[rows * cols].className, 'strip');
    deck.panel.children.slice(rows * cols + 1).forEach((dial, index) => {
      assert.equal(dial.className, 'dial');
      assert.equal(dial.style.gridRow, undefined);
      assert.equal(dial.style.gridColumnStart, id === 'plus-xl' && index === 0 ? 2 : undefined);
    });
  }
});
