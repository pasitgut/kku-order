import { test } from "node:test";
import assert from "node:assert/strict";
import { cronLabel } from "./cron-label.ts";

test("daily schedules read as a time of day", () => {
  assert.equal(cronLabel("0 5 * * *"), "ทุกวัน 05:00 น.");
  assert.equal(cronLabel("10 5 * * *"), "ทุกวัน 05:10 น.");
});

test("minute intervals read as every N minutes", () => {
  assert.equal(cronLabel("*/5 * * * *"), "ทุก 5 นาที");
  assert.equal(cronLabel("* * * * *"), "ทุก 1 นาที");
});

test("anything else is shown as the raw expression", () => {
  assert.equal(cronLabel("0 5 * * 1-5"), "0 5 * * 1-5");
  assert.equal(cronLabel("@every 1h"), "@every 1h");
});
