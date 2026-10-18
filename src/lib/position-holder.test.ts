import { test } from "node:test";
import assert from "node:assert/strict";
import { linkedRowsNote, teacherPayload } from "./position-holder.ts";

test("teacherPayload trims every field and leaves out an empty executive position", () => {
  const payload = teacherPayload({ prefix: " อ. ", firstName: " สมชาย ", lastName: "ใจดี ", positionTitle: "", department: "", email: "", phone: "", managePosition: "  ", managePositionSince: "1 ต.ค. 2568" });
  assert.equal(payload.prefix, "อ.");
  assert.equal(payload.firstName, "สมชาย");
  assert.equal(payload.lastName, "ใจดี");
  assert.equal("managePosition" in payload, false);
  assert.equal("managePositionSince" in payload, false);
});

test("teacherPayload keeps the executive position with its start date", () => {
  const payload = teacherPayload({ prefix: "", firstName: "ก", lastName: "ข", positionTitle: "", department: "", email: "", phone: "", managePosition: " ผู้ช่วยคณบดีฝ่ายวิชาการ ", managePositionSince: " 1 ต.ค. 2568 " });
  assert.equal(payload.managePosition, "ผู้ช่วยคณบดีฝ่ายวิชาการ");
  assert.equal(payload.managePositionSince, "1 ต.ค. 2568");
});

test("linkedRowsNote says how many waiting rows were filled", () => {
  assert.equal(linkedRowsNote(0), "");
  assert.equal(linkedRowsNote(3), " และผูกให้ 3 แถวในเอกสารที่รอยืนยันแล้ว");
});
