import type { Appointment } from "./api.ts";
import { departmentOf, groupByDepartment, personLabel, rosterName } from "./member-review.ts";

// หนึ่งฝ่ายในคำสั่ง: หน้าที่ตามคำสั่ง (เก็บซ้ำในทุกแถวของฝ่าย) กับหน้าที่ที่ผู้ตรวจเขียนเพิ่ม
export type DepartmentDuty = {
  name: string;
  count: number;
  members: string[];
  duty: string;
  pageNo: number;
  extras: string[];
};

function splitLines(text: string | undefined) {
  return (text ?? "").split("\n").map((line) => line.trim()).filter(Boolean);
}

function memberName(row: Appointment) {
  return rosterName(row).trim() || personLabel(row);
}

export function departmentDuties(rows: Appointment[]): DepartmentDuty[] {
  return groupByDepartment(rows).map(([name, group]) => {
    const source = group.find((row) => row.responsibilities.trim());
    const extras = group.find((row) => (row.extraDuties ?? "").trim());
    // ฝ่ายชื่อเดียวกันอาจมาจากหลายกลุ่มในคำสั่งที่หน้าที่ต่างกัน แสดงทุกข้อความ การแก้จึงไม่ทับข้อความที่มองไม่เห็น
    const duties = [...new Set(group.map((row) => row.responsibilities.trim()).filter(Boolean))];
    return {
      name,
      count: group.length,
      members: group.map(memberName),
      duty: duties.join("\n"),
      pageNo: source?.pageNo ?? group[0]?.pageNo ?? 0,
      extras: splitLines(extras?.extraDuties),
    };
  });
}

// setDepartmentDuty แก้ข้อความตามคำสั่งของทั้งฝ่าย ทุกแถวในฝ่ายได้ข้อความเดียวกัน รายงานจึงตรงกันทุกคน
export function setDepartmentDuty(rows: Appointment[], department: string, text: string): Appointment[] {
  return rows.map((row) => departmentOf(row) === department ? { ...row, responsibilities: text.trim() } : row);
}

function setExtras(rows: Appointment[], department: string, extras: string[]): Appointment[] {
  return rows.map((row) => departmentOf(row) === department ? { ...row, extraDuties: extras.join("\n") } : row);
}

function extrasOf(rows: Appointment[], department: string) {
  return departmentDuties(rows).find((item) => item.name === department)?.extras ?? [];
}

// addDepartmentExtra เพิ่มหน้าที่หนึ่งข้อให้ทั้งฝ่าย ข้อความว่างหรือซ้ำไม่เพิ่ม
export function addDepartmentExtra(rows: Appointment[], department: string, line: string): Appointment[] {
  const text = line.trim();
  const extras = extrasOf(rows, department);
  if (!text || extras.includes(text)) return rows;
  return setExtras(rows, department, [...extras, text]);
}

export function removeDepartmentExtra(rows: Appointment[], department: string, index: number): Appointment[] {
  return setExtras(rows, department, extrasOf(rows, department).filter((_, at) => at !== index));
}
