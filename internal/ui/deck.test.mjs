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
    ['mobile', 2, 4, false, false, 'face'],
    ['corsair-kb', 6, 1, false, false, 'face'],
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
