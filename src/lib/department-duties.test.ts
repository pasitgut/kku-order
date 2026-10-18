import { test } from "node:test";
import assert from "node:assert/strict";
import type { Appointment } from "./api.ts";
import { addDepartmentExtra, departmentDuties, removeDepartmentExtra, setDepartmentDuty } from "./department-duties.ts";

const row = (id: number, department: string, extra: Partial<Appointment> = {}): Appointment => ({
  id, documentId: 1, fullName: `คน ${id}`, position: "", department, committeeRole: "กรรมการ", responsibilities: "", confidence: 1, pageNo: id, boundingBox: "null", verified: false, ...extra,
});

const rows = [
  row(1, "ฝ่ายอำนวยการ", { responsibilities: "ให้คำปรึกษา", pageNo: 1 }),
  row(2, "ฝ่ายอำนวยการ", { responsibilities: "ให้คำปรึกษา", pageNo: 1 }),
  row(3, "ฝ่ายดำเนินงาน", { extraDuties: "จัดทำรายงาน\nดูแลงบ" }),
  row(4, "", { fullName: "", position: "คณบดี" }),
];

test("departmentDuties groups rows in document order with their duty and what reviewers added", () => {
  const departments = departmentDuties(rows);
  assert.deepEqual(departments.map((item) => [item.name, item.count, item.duty, item.extras]), [
    ["ฝ่ายอำนวยการ", 2, "ให้คำปรึกษา", []],
    ["ฝ่ายดำเนินงาน", 1, "", ["จัดทำรายงาน", "ดูแลงบ"]],
    ["ไม่ระบุฝ่าย", 1, "", []],
  ]);
  assert.equal(departments[0].pageNo, 1);
  assert.deepEqual(departments[2].members, ["คณบดี (โดยตำแหน่ง)"]);
});

test("rows of one department with different duties show every duty, so editing never hides one", () => {
  const mixed = [row(1, "ฝ่ายร่วม", { responsibilities: "งาน ก" }), row(2, "ฝ่ายร่วม", { responsibilities: "งาน ข" }), row(3, "ฝ่ายร่วม", { responsibilities: "งาน ก" })];
  assert.equal(departmentDuties(mixed)[0].duty, "งาน ก\nงาน ข");
});

test("editing a department's duty changes every row in that department only", () => {
  const next = setDepartmentDuty(rows, "ฝ่ายอำนวยการ", "  ให้คำปรึกษาและกำกับดูแล ");
  assert.deepEqual(next.map((item) => item.responsibilities), ["ให้คำปรึกษาและกำกับดูแล", "ให้คำปรึกษาและกำกับดูแล", "", ""]);
});

test("extra duties are added once per department and can be removed", () => {
  let next = addDepartmentExtra(rows, "ฝ่ายอำนวยการ", " ดูแลงบประมาณ ");
  next = addDepartmentExtra(next, "ฝ่ายอำนวยการ", "ดูแลงบประมาณ");
  next = addDepartmentExtra(next, "ฝ่ายอำนวยการ", "   ");
  assert.deepEqual(next.slice(0, 2).map((item) => item.extraDuties), ["ดูแลงบประมาณ", "ดูแลงบประมาณ"]);
  next = removeDepartmentExtra(next, "ฝ่ายดำเนินงาน", 0);
  assert.equal(next[2].extraDuties, "ดูแลงบ");
});
