import { test } from "node:test";
import assert from "node:assert/strict";
import type { Appointment } from "./api.ts";
import { countByFilter, filterMembers, groupByDepartment, matchState, rosterRows, summarizeDepartments } from "./member-review.ts";

function appointment(overrides: Partial<Appointment>): Appointment {
  return {
    id: 1,
    documentId: 10,
    fullName: "ผศ.ดร.สมชาย ใจดี",
    position: "ผู้ช่วยศาสตราจารย์",
    department: "คณะกรรมการบริหารหลักสูตร",
    committeeRole: "ประธานกรรมการ",
    responsibilities: "",
    confidence: 0.97,
    pageNo: 1,
    boundingBox: "",
    verified: false,
    ...overrides,
  };
}

const rows: Appointment[] = [
  appointment({ id: 1, directoryUserId: 11, nameMatchMethod: "EXACT", verified: true }),
  appointment({ id: 2, fullName: "อ.ธนพล แก้วมณี", directoryUserId: 12, nameMatchMethod: "FUZZY", nameMatchScore: 0.86 }),
  appointment({ id: 3, fullName: "อ.ณัฐวุฒิ สุขสวัสดิ์", nameMatchMethod: "AMBIGUOUS", candidates: [{ userId: 41, fullName: "ณัฐวุฒิ สุขสวัสดิ์", positionTitle: "อาจารย์", score: 1 }, { userId: 42, fullName: "ณัฐวุฒิ สุขสวัสดิ์", positionTitle: "นักวิชาการคอมพิวเตอร์", score: 1 }] }),
  appointment({ id: 4, fullName: "ศ.ดร.ประเสริฐ วงศ์ใหญ่", department: "ที่ปรึกษา", directoryUserId: 14, nameMatchMethod: "EXACT" }),
  appointment({ id: 5, fullName: "นางสาวพิมพ์ชนก อินทร์แก้ว", department: "", position: "นักวิชาการศึกษา", nameMatchMethod: "NOT_FOUND" }),
];

test("matchState keeps unlinked names out of the good tone", () => {
  assert.equal(matchState(rows[0]).tone, "good");
  assert.equal(matchState(rows[1]).tone, "warning");
  assert.equal(matchState(rows[1]).label, "ใกล้เคียง 86%");
  assert.equal(matchState(rows[2]).tone, "danger");
  assert.equal(matchState(rows[2]).label, "ชื่อซ้ำ 2 คน");
  assert.equal(matchState(rows[4]).label, "ไม่พบในระบบ");
});

test("countByFilter follows the reviewer filters", () => {
  assert.deepEqual(countByFilter(rows), { all: 5, warning: 3, pending: 1, verified: 1 });
});

test("filterMembers combines filter, department and query", () => {
  assert.deepEqual(filterMembers(rows, { filter: "warning", department: "", query: "" }).map((row) => row.id), [2, 3, 5]);
  assert.deepEqual(filterMembers(rows, { filter: "all", department: "ที่ปรึกษา", query: "" }).map((row) => row.id), [4]);
  assert.deepEqual(filterMembers(rows, { filter: "all", department: "", query: "นักวิชาการ" }).map((row) => row.id), [5]);
  assert.deepEqual(filterMembers(rows, { filter: "all", department: "ไม่ระบุฝ่าย", query: "" }).map((row) => row.id), [5]);
});

test("summarizeDepartments counts unlinked names per department in document order", () => {
  assert.deepEqual(summarizeDepartments(rows), [
    { name: "คณะกรรมการบริหารหลักสูตร", total: 3, unlinked: 1 },
    { name: "ที่ปรึกษา", total: 1, unlinked: 0 },
    { name: "ไม่ระบุฝ่าย", total: 1, unlinked: 1 },
  ]);
});

test("groupByDepartment keeps the order names appear in", () => {
  assert.deepEqual(groupByDepartment(rows).map(([name, group]) => [name, group.map((row) => row.id)]), [
    ["คณะกรรมการบริหารหลักสูตร", [1, 2, 3]],
    ["ที่ปรึกษา", [4]],
    ["ไม่ระบุฝ่าย", [5]],
  ]);
});

test("rosterRows lists every name with department and position only", () => {
  const roster = rosterRows([rows[3], rows[0], rows[4]]);
  assert.deepEqual(roster, [
    { id: 4, no: 1, fullName: "ศ.ดร.ประเสริฐ วงศ์ใหญ่", department: "ที่ปรึกษา", position: "ผู้ช่วยศาสตราจารย์" },
    { id: 1, no: 2, fullName: "ผศ.ดร.สมชาย ใจดี", department: "คณะกรรมการบริหารหลักสูตร", position: "ผู้ช่วยศาสตราจารย์" },
    { id: 5, no: 3, fullName: "นางสาวพิมพ์ชนก อินทร์แก้ว", department: "ไม่ระบุฝ่าย", position: "นักวิชาการศึกษา" },
  ]);
});
