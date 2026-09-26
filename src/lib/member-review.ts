import type { Appointment } from "./api";

export type MemberFilter = "all" | "warning" | "pending" | "verified";
export type MatchTone = "good" | "warning" | "danger";
export type MatchState = { label: string; detail: string; tone: MatchTone };
export type DepartmentSummary = { name: string; total: number; unlinked: number };
export type RosterRow = { id: number; no: number; fullName: string; department: string; position: string };

export const NO_DEPARTMENT = "ไม่ระบุฝ่าย";

// วิธีจับคู่เหล่านี้หาคนเจอแล้ว เหลือแค่เจ้าหน้าที่กดยืนยันว่าใช่คนนั้นจริง
const resolvedMethods = new Set(["ALIAS", "EXACT", "FUZZY", "MANUAL"]);

export function isLinked(row: Appointment) {
  return Boolean(row.directoryUserId);
}

export function departmentOf(row: Appointment) {
  return row.department?.trim() || NO_DEPARTMENT;
}

export function matchState(row: Appointment): MatchState {
  const method = row.nameMatchMethod ?? "";
  if (!isLinked(row)) {
    if (resolvedMethods.has(method)) return { label: "พบชื่อตรง รอผูก", detail: "พบชื่อที่ตรงกัน กดเพื่อผูก", tone: "warning" };
    if (method === "AMBIGUOUS") return { label: `ชื่อซ้ำ ${row.candidates?.length || "หลาย"} คน`, detail: "มีชื่อซ้ำกันหลายคน ต้องเลือกเอง", tone: "danger" };
    if (method === "DIRECTORY_EMPTY") return { label: "ยังไม่มีฐานบุคลากร", detail: "ยังไม่มีฐานข้อมูลบุคลากร กรุณา sync ก่อน", tone: "danger" };
    return { label: "ไม่พบในระบบ", detail: "ไม่พบรายชื่อในระบบ", tone: "danger" };
  }
  if (method === "MANUAL") return { label: "เจ้าหน้าที่เลือกเอง", detail: "เจ้าหน้าที่เลือกเอง", tone: "good" };
  if (method === "ALIAS") return { label: "เคยผูกชื่อนี้ไว้แล้ว", detail: "เคยผูกชื่อนี้ไว้แล้ว", tone: "good" };
  if (method === "FUZZY") {
    const percent = Math.round((row.nameMatchScore ?? 0) * 100);
    return { label: `ใกล้เคียง ${percent}%`, detail: `ชื่อใกล้เคียงในระบบ ${percent}%`, tone: "warning" };
  }
  return { label: "พบรายชื่อในระบบ", detail: "พบรายชื่อในระบบ", tone: "good" };
}

export function matchesFilter(row: Appointment, filter: MemberFilter) {
  const tone = matchState(row).tone;
  if (filter === "warning") return tone !== "good";
  if (filter === "pending") return !row.verified && tone === "good";
  if (filter === "verified") return row.verified;
  return true;
}

export function countByFilter(rows: Appointment[]): Record<MemberFilter, number> {
  const count = (filter: MemberFilter) => rows.filter((row) => matchesFilter(row, filter)).length;
  return { all: rows.length, warning: count("warning"), pending: count("pending"), verified: count("verified") };
}

export function filterMembers(rows: Appointment[], options: { filter: MemberFilter; department: string; query: string }) {
  const query = options.query.trim().toLowerCase();
  return rows.filter((row) => {
    const matchesQuery = !query || [row.fullName, row.position, row.committeeRole, row.department].join(" ").toLowerCase().includes(query);
    const matchesDepartment = !options.department || departmentOf(row) === options.department;
    return matchesQuery && matchesDepartment && matchesFilter(row, options.filter);
  });
}

export function summarizeDepartments(rows: Appointment[]): DepartmentSummary[] {
  const totals = new Map<string, DepartmentSummary>();
  for (const row of rows) {
    const name = departmentOf(row);
    const entry = totals.get(name) ?? { name, total: 0, unlinked: 0 };
    entry.total += 1;
    if (!isLinked(row)) entry.unlinked += 1;
    totals.set(name, entry);
  }
  return [...totals.values()];
}

export function groupByDepartment(rows: Appointment[]): [string, Appointment[]][] {
  const groups = new Map<string, Appointment[]>();
  for (const row of rows) {
    const name = departmentOf(row);
    groups.set(name, [...(groups.get(name) ?? []), row]);
  }
  return [...groups.entries()];
}

// รายชื่อสำหรับอ่านอย่างเดียว เรียงตามฝ่ายที่ปรากฏในเอกสาร
export function rosterRows(rows: Appointment[]): RosterRow[] {
  return groupByDepartment(rows)
    .flatMap(([, group]) => group)
    .map((row, index) => ({ id: row.id, no: index + 1, fullName: row.fullName, department: departmentOf(row), position: row.position }));
}
