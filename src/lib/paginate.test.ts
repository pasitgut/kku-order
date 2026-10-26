import { test } from "node:test";
import assert from "node:assert/strict";
import { paginate } from "./paginate.ts";

const items = Array.from({ length: 32 }, (_, index) => index + 1);

test("paginate slices one page and reports the visible range", () => {
  assert.deepEqual(paginate(items, 1, 15), { items: items.slice(0, 15), page: 1, pageCount: 3, from: 1, to: 15, total: 32 });
  assert.deepEqual(paginate(items, 3, 15), { items: [31, 32], page: 3, pageCount: 3, from: 31, to: 32, total: 32 });
});

test("paginate clamps a page that no longer exists after filtering", () => {
  assert.equal(paginate(items, 9, 15).page, 3);
  assert.equal(paginate(items, 0, 15).page, 1);
});

test("paginate handles an empty list", () => {
  assert.deepEqual(paginate([], 2, 15), { items: [], page: 1, pageCount: 1, from: 0, to: 0, total: 0 });
});
