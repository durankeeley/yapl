import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  Buttons, gridMove, listMove, cycle, axisDirection, actionForButton, actionForKey,
  repeatDue, REPEAT_DELAY_MS, REPEAT_INTERVAL_MS, STICK_DEADZONE,
  formatBytes, jobPercent, initials, tileHue,
} from './nav.js';

test('given tile 1 of a 3-column grid when moving down then focus goes to tile 4', () => {
  assert.equal(gridMove(0, 9, 3, 'down'), 3);
});

test('given the last tile of a row when moving right then focus stays (no wrap)', () => {
  assert.equal(gridMove(2, 9, 3, 'right'), 2);
});

test('given the first tile of a row when moving left then focus stays', () => {
  assert.equal(gridMove(3, 9, 3, 'left'), 3);
});

test('given the top row when moving up then focus stays', () => {
  assert.equal(gridMove(1, 9, 3, 'up'), 1);
});

test('given a short last row when moving down from above an empty cell then focus lands on the last tile', () => {
  // 3 columns, 7 tiles: last row has only tile 7 (index 6); index 5 has nothing below it
  assert.equal(gridMove(5, 7, 3, 'down'), 6);
});

test('given the last row when moving down then focus stays', () => {
  assert.equal(gridMove(6, 7, 3, 'down'), 6);
});

test('given an empty grid then the index stays at zero', () => {
  assert.equal(gridMove(0, 0, 3, 'down'), 0);
});

test('given a single column then up and down step one tile', () => {
  assert.equal(gridMove(1, 3, 1, 'down'), 2);
  assert.equal(gridMove(1, 3, 1, 'up'), 0);
});

test('given tabs when cycling past either end then it wraps', () => {
  assert.equal(cycle(2, 3, 1), 0);
  assert.equal(cycle(0, 3, -1), 2);
});

test('given a stick inside the dead zone then there is no direction', () => {
  assert.equal(axisDirection(STICK_DEADZONE / 2, 0), null);
});

test('given a stick pushed mostly right then the direction is right', () => {
  assert.equal(axisDirection(0.9, 0.3), 'right');
});

test('given a stick pushed mostly up then the direction is up', () => {
  assert.equal(axisDirection(0.2, -0.95), 'up');
});

test('given standard gamepad buttons then they map to console actions', () => {
  assert.equal(actionForButton(Buttons.A), 'select');
  assert.equal(actionForButton(Buttons.B), 'back');
  assert.equal(actionForButton(Buttons.LB), 'prevTab');
  assert.equal(actionForButton(Buttons.RB), 'nextTab');
  assert.equal(actionForButton(Buttons.START), 'play');
  assert.equal(actionForButton(Buttons.UP), 'up');
  assert.equal(actionForButton(99), null);
});

test('given keyboard keys then they map to the same actions', () => {
  assert.equal(actionForKey('ArrowLeft'), 'left');
  assert.equal(actionForKey('Enter'), 'select');
  assert.equal(actionForKey('Escape'), 'back');
  assert.equal(actionForKey('Backspace'), 'back');
  assert.equal(actionForKey('q'), 'prevTab');
  assert.equal(actionForKey('E'), 'nextTab');
  assert.equal(actionForKey(' '), 'play');
  assert.equal(actionForKey('z'), null);
});

test('given a held direction then it repeats only after the delay and then at the interval', () => {
  const pressed = 1000;
  assert.equal(repeatDue(pressed, pressed, pressed + REPEAT_DELAY_MS - 1), false);
  assert.equal(repeatDue(pressed, pressed, pressed + REPEAT_DELAY_MS), true);
  const last = pressed + REPEAT_DELAY_MS;
  assert.equal(repeatDue(pressed, last, last + REPEAT_INTERVAL_MS - 1), false);
  assert.equal(repeatDue(pressed, last, last + REPEAT_INTERVAL_MS), true);
});

test('given byte counts then they are shown in human units', () => {
  assert.equal(formatBytes(0), '0 B');
  assert.equal(formatBytes(1536), '1.5 KB');
  assert.equal(formatBytes(5 * 1024 ** 3), '5.0 GB');
});

test('given a job with a known total then its percent is computed and clamped', () => {
  assert.equal(jobPercent({ done: 50, total: 200 }), 25);
  assert.equal(jobPercent({ done: 300, total: 200 }), 100);
  assert.equal(jobPercent({ done: 10, total: -1 }), null);
});

test('given titles then tiles get up to two initials', () => {
  assert.equal(initials('Half-Life'), 'HL');
  assert.equal(initials('Need for Speed: Most Wanted'), 'NF');
  assert.equal(initials('doom'), 'D');
  assert.equal(initials(''), '?');
});

test('given a name then its tile hue is stable and within the colour wheel', () => {
  assert.equal(tileHue('Doom'), tileHue('Doom'));
  const h = tileHue('Quake');
  assert.ok(h >= 0 && h < 360);
});

test('given dialog buttons that wrap when moving down then focus moves to the next button', () => {
  assert.equal(listMove(2, 4, 'down'), 3);
  assert.equal(listMove(3, 4, 'down'), 3);
  assert.equal(listMove(0, 4, 'up'), 0);
  assert.equal(listMove(1, 4, 'left'), 0);
  assert.equal(listMove(0, 0, 'right'), 0);
});
