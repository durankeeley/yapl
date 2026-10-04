// Pure navigation and input logic for the console UI. Everything here is
// free of the DOM so it can be tested with `node --test`.

// Standard Gamepad API button indices.
export const Buttons = Object.freeze({
  A: 0, B: 1, X: 2, Y: 3, LB: 4, RB: 5, SELECT: 8, START: 9,
  UP: 12, DOWN: 13, LEFT: 14, RIGHT: 15,
});

// Left-stick travel below this is ignored, so a resting stick never drifts.
export const STICK_DEADZONE = 0.5;
// A held direction waits this long before repeating, then repeats every
// REPEAT_INTERVAL_MS — the same feel as a console menu.
export const REPEAT_DELAY_MS = 350;
export const REPEAT_INTERVAL_MS = 120;

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];
const BYTES_PER_UNIT = 1024;
const PERCENT = 100;
const MAX_INITIALS = 2;
const HUE_RANGE = 360;
const HASH_MULTIPLIER = 31;

const buttonActions = new Map([
  [Buttons.A, 'select'],
  [Buttons.B, 'back'],
  [Buttons.X, 'details'],
  [Buttons.LB, 'prevTab'],
  [Buttons.RB, 'nextTab'],
  [Buttons.START, 'play'],
  [Buttons.UP, 'up'],
  [Buttons.DOWN, 'down'],
  [Buttons.LEFT, 'left'],
  [Buttons.RIGHT, 'right'],
]);

const keyActions = new Map([
  ['ArrowUp', 'up'],
  ['ArrowDown', 'down'],
  ['ArrowLeft', 'left'],
  ['ArrowRight', 'right'],
  ['Enter', 'select'],
  ['Escape', 'back'],
  ['Backspace', 'back'],
  ['q', 'prevTab'],
  ['e', 'nextTab'],
  ['x', 'details'],
  [' ', 'play'],
]);

export function actionForButton(index) {
  return buttonActions.get(index) ?? null;
}

export function actionForKey(key) {
  return keyActions.get(key.length === 1 ? key.toLowerCase() : key) ?? null;
}

// gridMove returns the tile index after moving in a grid of `count` tiles
// laid out in `columns`. Rows never wrap; moving down from above an empty
// cell in a short last row lands on the last tile.
export function gridMove(index, count, columns, direction) {
  if (count === 0) return 0;
  const cols = Math.max(1, columns);
  const row = Math.floor(index / cols);
  const lastRow = Math.floor((count - 1) / cols);
  switch (direction) {
    case 'left':
      return index % cols === 0 ? index : index - 1;
    case 'right':
      return index % cols === cols - 1 || index === count - 1 ? index : index + 1;
    case 'up':
      return row === 0 ? index : index - cols;
    case 'down':
      if (row === lastRow) return index;
      return Math.min(index + cols, count - 1);
    default:
      return index;
  }
}

// listMove steps through a row of buttons that may wrap onto several
// lines: left/up go back one, right/down go forward one, without wrapping.
export function listMove(index, count, direction) {
  if (count === 0) return 0;
  if (direction === 'left' || direction === 'up') return Math.max(0, index - 1);
  if (direction === 'right' || direction === 'down') return Math.min(count - 1, index + 1);
  return index;
}

// cycle steps through `count` items with wrap-around (used for tabs).
export function cycle(index, count, delta) {
  return (index + delta + count) % count;
}

export function axisDirection(x, y, deadzone = STICK_DEADZONE) {
  if (Math.abs(x) < deadzone && Math.abs(y) < deadzone) return null;
  if (Math.abs(x) >= Math.abs(y)) return x > 0 ? 'right' : 'left';
  return y > 0 ? 'down' : 'up';
}

export function repeatDue(pressedAt, lastFiredAt, now) {
  if (lastFiredAt === pressedAt) return now - pressedAt >= REPEAT_DELAY_MS;
  return now - lastFiredAt >= REPEAT_INTERVAL_MS;
}

export function formatBytes(bytes) {
  let value = bytes;
  let unit = 0;
  while (value >= BYTES_PER_UNIT && unit < BYTE_UNITS.length - 1) {
    value /= BYTES_PER_UNIT;
    unit += 1;
  }
  return unit === 0 ? `${value} ${BYTE_UNITS[0]}` : `${value.toFixed(1)} ${BYTE_UNITS[unit]}`;
}

export function jobPercent(job) {
  if (!job || job.total <= 0) return null;
  return Math.min(PERCENT, Math.round((job.done / job.total) * PERCENT));
}

export function initials(title) {
  const words = (title || '').split(/[^\p{L}\p{N}]+/u).filter(Boolean);
  if (words.length === 0) return '?';
  return words.slice(0, MAX_INITIALS).map((w) => w[0].toUpperCase()).join('');
}

// tileHue gives games without cover art a stable, distinct colour.
export function tileHue(name) {
  let hash = 0;
  for (const ch of name) hash = (hash * HASH_MULTIPLIER + ch.codePointAt(0)) % HUE_RANGE;
  return hash;
}
