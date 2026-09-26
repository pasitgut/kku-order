import { test } from "node:test";
import assert from "node:assert/strict";
import { clampOffset, coverScale, sourceRect } from "./photo-crop.ts";

test("coverScale fills the square viewport with the short side", () => {
  assert.equal(coverScale(1000, 500, 250), 0.5);
  assert.equal(coverScale(400, 800, 200), 0.5);
});

test("clampOffset keeps the image covering the viewport", () => {
  // 1000x500 at scale 0.5 shows 500x250 in a 250 viewport: 125 px of slack each way horizontally, none vertically.
  assert.deepEqual(clampOffset(1000, 500, 250, 1, { x: 400, y: 30 }), { x: 125, y: 0 });
  assert.deepEqual(clampOffset(1000, 500, 250, 1, { x: -400, y: -30 }), { x: -125, y: 0 });
});

test("sourceRect maps the visible square back to image pixels", () => {
  assert.deepEqual(sourceRect(1000, 500, 250, 1, { x: 0, y: 0 }), { x: 250, y: 0, size: 500 });
  assert.deepEqual(sourceRect(1000, 500, 250, 2, { x: 0, y: 0 }), { x: 375, y: 125, size: 250 });
  // Dragging the image right reveals its left edge.
  assert.deepEqual(sourceRect(1000, 500, 250, 1, { x: 125, y: 0 }), { x: 0, y: 0, size: 500 });
});
