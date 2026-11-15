import { test } from "node:test";
import assert from "node:assert/strict";
import { sourceLabel } from "./current-positions.ts";

test("sourceLabel names where the holder came from", () => {
  assert.equal(sourceLabel("SYNC"), "ระบบวิทยาลัย");
  assert.equal(sourceLabel("MANUAL"), "กรอกเอง");
  assert.equal(sourceLabel("LEARNED"), "จากการตรวจเอกสาร");
  assert.equal(sourceLabel("OTHER"), "OTHER");
});
