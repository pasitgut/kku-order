import { test } from "node:test";
import assert from "node:assert/strict";
import { hasNextPage } from "./pagination.ts";

test("no next page when the last page is exactly full", () => {
  assert.equal(hasNextPage(1, 25, 25), false);
  assert.equal(hasNextPage(2, 25, 50), false);
});

test("next page exists while rows remain beyond this page", () => {
  assert.equal(hasNextPage(1, 25, 26), true);
  assert.equal(hasNextPage(2, 25, 51), true);
});

test("no next page for an empty or short list", () => {
  assert.equal(hasNextPage(1, 25, 0), false);
  assert.equal(hasNextPage(1, 25, 7), false);
});
