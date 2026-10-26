import { test } from "node:test";
import assert from "node:assert/strict";
import type { Appointment } from "./api.ts";
import { countByFilter, filterMembers, groupByDepartment, matchState, pageEntries, rosterRows, summarizeDepartments } from "./member-review.ts";

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

test("pageEntries keeps every page to the same number of lines, headers included", () => {
  const many: Appointment[] = [
    ...Array.from({ length: 5 }, (_, index) => appointment({ id: 100 + index, department: "ฝ่าย ก" })),
    ...Array.from({ length: 6 }, (_, index) => appointment({ id: 200 + index, department: "ฝ่าย ข" })),
  ];
  const pages = [1, 2].map((page) => pageEntries(groupByDepartment(many), page, 6));
  // หน้า 1: หัว ก + 5 คน · หน้า 2: หัว ข + 5 คน · หน้า 3: หัว ข (ต่อ) + 1 คน
  assert.deepEqual(pages[0].entries.map((entry) => entry.kind === "header" ? `H:${entry.department}${entry.continued ? "+" : ""}` : entry.row.id), ["H:ฝ่าย ก", 100, 101, 102, 103, 104]);
  assert.deepEqual(pages[1].entries.map((entry) => entry.kind === "header" ? `H:${entry.department}${entry.continued ? "+" : ""}` : entry.row.id), ["H:ฝ่าย ข", 200, 201, 202, 203, 204]);
  const last = pageEntries(groupByDepartment(many), 3, 6);
  assert.deepEqual(last.entries.map((entry) => entry.kind === "header" ? `H:${entry.department}${entry.continued ? "+" : ""}` : entry.row.id), ["H:ฝ่าย ข+", 205]);
  assert.equal(last.pageCount, 3);
  assert.deepEqual({ from: last.from, to: last.to, total: last.total }, { from: 11, to: 11, total: 11 });
});

test("pageEntries clamps out-of-range pages and handles no rows", () => {
  assert.equal(pageEntries(groupByDepartment(rows), 99, 15).page, 1);
  assert.deepEqual(pageEntries([], 1, 15), { entries: [], page: 1, pageCount: 1, from: 0, to: 0, total: 0 });
});
