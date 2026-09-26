import { test } from "node:test";
import assert from "node:assert/strict";
import { initials } from "./person.ts";

test("initials use the first letter of first and last name", () => {
  assert.equal(initials("กฤตยชญ์ มัตกิจ"), "กม");
  assert.equal(initials("John Smith"), "JS");
});

test("initials skip academic titles and leading Thai vowels", () => {
  assert.equal(initials("ผศ.ดร.สมชาย ใจดี"), "สจ");
  assert.equal(initials("นางสาวพิมพ์ชนก อินทร์แก้ว"), "พอ");
  assert.equal(initials("อ. เอกชัย ไชยวงศ์"), "อช");
});

test("initials fall back gracefully", () => {
  assert.equal(initials("local-user"), "LO");
  assert.equal(initials(""), "?");
});
