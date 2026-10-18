import { test } from "node:test";
import assert from "node:assert/strict";
import { appointmentStateLabel, fiscalYearRange, moveSelection, periodRange, rememberSearch } from "./search.ts";

test("the fiscal year runs October to September and is named by its Buddhist year", () => {
  assert.deepEqual(fiscalYearRange(new Date(2026, 9, 18)), { from: "2026-10-01", to: "2027-09-30", label: "ปีงบ 2570" });
  assert.deepEqual(fiscalYearRange(new Date(2026, 8, 30)), { from: "2025-10-01", to: "2026-09-30", label: "ปีงบ 2569" });
  assert.deepEqual(fiscalYearRange(new Date(2026, 9, 18), -1), { from: "2025-10-01", to: "2026-09-30", label: "ปีงบ 2569" });
});

test("periodRange turns the chosen period into issue-date bounds", () => {
  const today = new Date(2026, 2, 5);
  assert.deepEqual(periodRange("all", today), {});
  assert.deepEqual(periodRange("this", today), { from: "2025-10-01", to: "2026-09-30" });
  assert.deepEqual(periodRange("last", today), { from: "2024-10-01", to: "2025-09-30" });
  assert.deepEqual(periodRange("custom", today, { from: "2026-01-01", to: "" }), { from: "2026-01-01" });
});

test("rememberSearch keeps the newest first without repeats", () => {
  const one = { kind: "person" as const, id: 1, label: "ก" };
  const two = { kind: "order" as const, id: 1, label: "1/2569" };
  let list = rememberSearch([], one);
  list = rememberSearch(list, two);
  list = rememberSearch(list, one);
  assert.deepEqual(list, [one, two]);
  const many = Array.from({ length: 10 }, (_, id) => ({ kind: "person" as const, id, label: String(id) })).reduce(rememberSearch, [] as ReturnType<typeof rememberSearch>);
  assert.equal(many.length, 6);
  assert.equal(many[0].id, 9);
});

test("moveSelection wraps around the result list", () => {
  assert.equal(moveSelection(-1, 1, 3), 0);
  assert.equal(moveSelection(2, 1, 3), 0);
  assert.equal(moveSelection(0, -1, 3), 2);
  assert.equal(moveSelection(-1, -1, 3), 2);
  assert.equal(moveSelection(0, 1, 0), -1);
});

test("appointment states read as Thai labels", () => {
  assert.equal(appointmentStateLabel("ACTIVE"), "มีผล");
  assert.equal(appointmentStateLabel("EXPIRING"), "ใกล้หมดวาระ");
  assert.equal(appointmentStateLabel("EXPIRED"), "หมดวาระ");
  assert.equal(appointmentStateLabel("CANCELLED"), "ยกเลิก");
});
