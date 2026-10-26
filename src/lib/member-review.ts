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

export type MemberEntry = { kind: "header"; department: string; continued: boolean } | { kind: "row"; row: Appointment };
export type MemberPage = { entries: MemberEntry[]; page: number; pageCount: number; from: number; to: number; total: number };

// หัวข้อฝ่ายนับเป็นหนึ่งบรรทัด ทุกหน้าจึงสูงเท่ากัน ฝ่ายที่ต่อข้ามหน้าจะมีหัวข้อ "(ต่อ)" ซ้ำ
export function pageEntries(groups: [string, Appointment[]][], page: number, linesPerPage: number): MemberPage {
  const pages: MemberEntry[][] = [];
  let current: MemberEntry[] = [];
  const flush = () => { if (current.length) pages.push(current); current = []; };
  for (const [department, rows] of groups) {
    rows.forEach((row, index) => {
      // หัวข้อต้องมีแถวตามอย่างน้อยหนึ่งแถว จึงขึ้นหน้าใหม่ถ้าเหลือที่ไม่พอ
      const needsHeader = index === 0 || current.length === 0;
      if (current.length + (needsHeader ? 2 : 1) > linesPerPage) flush();
      if (index === 0 || current.length === 0) current.push({ kind: "header", department, continued: index > 0 });
      current.push({ kind: "row", row });
    });
  }
  flush();
  const total = groups.reduce((sum, [, rows]) => sum + rows.length, 0);
  const pageCount = Math.max(1, pages.length);
  const currentPage = Math.min(pageCount, Math.max(1, page));
  const entries = pages[currentPage - 1] ?? [];
  const before = pages.slice(0, currentPage - 1).reduce((sum, entriesOnPage) => sum + entriesOnPage.filter((entry) => entry.kind === "row").length, 0);
  const shown = entries.filter((entry) => entry.kind === "row").length;
  return { entries, page: currentPage, pageCount, from: shown ? before + 1 : 0, to: before + shown, total };
}
