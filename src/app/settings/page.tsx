"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowsClockwise, CaretLeft, CaretRight, Check, MagnifyingGlass } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { useToast } from "@/components/toast";
import { DirectoryUser, SyncRun, formatThaiDate, getDirectoryUsers, getSyncRuns, syncDirectoryUsers } from "@/lib/api";
import { PERMISSIONS, ROLES, can, roleLabel } from "@/lib/permissions";
import { initials } from "@/lib/person";

type SourceFilter = "all" | "SYNCED" | "MANUAL";
const PAGE_SIZE = 25;

export default function SettingsUsersPage() {
  const toast = useToast();
  const { me } = useMe();
  const [users, setUsers] = useState<DirectoryUser[]>([]);
  const [runs, setRuns] = useState<SyncRun[]>([]);
  const [query, setQuery] = useState("");
  const [source, setSource] = useState<SourceFilter>("all");
  const [page, setPage] = useState(1);
  const [syncing, setSyncing] = useState(false);
  const [error, setError] = useState("");

  const load = (search = "") => Promise.all([getDirectoryUsers(search), getSyncRuns()]).then(([userResponse, runResponse]) => { setUsers(userResponse.data); setRuns(runResponse.data); setError(""); }).catch((reason: Error) => setError(reason.message));
  useEffect(() => { Promise.all([getDirectoryUsers(), getSyncRuns()]).then(([userResponse, runResponse]) => { setUsers(userResponse.data); setRuns(runResponse.data); }).catch((reason: Error) => setError(reason.message)); }, []);

  const sync = async () => {
    setSyncing(true);
    try {
      const response = await syncDirectoryUsers();
      toast(`sync สำเร็จ ${response.data.fetched} รายการ · เพิ่ม ${response.data.created} · แก้ไข ${response.data.updated}`);
      await load(query);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "sync ไม่สำเร็จ", { kind: "error" });
    } finally {
      setSyncing(false);
    }
  };

  const manualCount = users.filter((user) => user.source === "MANUAL").length;
  const filtered = useMemo(() => users.filter((user) => source === "all" || (source === "MANUAL" ? user.source === "MANUAL" : user.source !== "MANUAL")), [users, source]);
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount);
  const visible = filtered.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);
  const lastRun = runs[0];
  const sourceFilters: [SourceFilter, string, number][] = [["all", "ทั้งหมด", users.length], ["SYNCED", "จาก sync", users.length - manualCount], ["MANUAL", "เพิ่มโดยเจ้าหน้าที่", manualCount]];

  const syncButton = can(me?.role ?? "", "sync") ? <button type="button" disabled={syncing} onClick={sync} className="focus-ring inline-flex h-12 items-center gap-2 rounded-md bg-white px-6 text-[15px] font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)] disabled:opacity-60"><ArrowsClockwise size={18} weight="bold" className={syncing ? "animate-spin" : ""} />{syncing ? "กำลัง sync..." : "sync บุคลากรตอนนี้"}</button> : undefined;

  return <SettingsFrame actions={syncButton} note="บัญชีและบทบาทมาจากระบบ SSO ของมหาวิทยาลัย รายชื่อบุคลากรมาจากการ sync และที่เจ้าหน้าที่เพิ่มเอง">
    {error && <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}

    <div role="status" className="card grid grid-cols-2 xl:grid-cols-4">
      {[["บุคลากรในฐานข้อมูล", users.length.toLocaleString("th-TH")], ["กำลังใช้งาน", users.filter((user) => user.isActive === "A").length.toLocaleString("th-TH")], ["เพิ่มโดยเจ้าหน้าที่", manualCount.toLocaleString("th-TH")]].map(([label, value]) => <div key={label} className="border-b border-r border-[var(--line-soft)] px-5 py-4 xl:border-b-0"><p className="text-xs text-[var(--muted)]">{label}</p><p className="mt-1 text-2xl font-semibold text-[var(--navy)]">{value}</p></div>)}
      <div className="px-5 py-4"><p className="text-xs text-[var(--muted)]">sync ล่าสุด</p><p className="mt-2 flex items-center gap-2 text-[15px] font-medium text-[var(--navy)]">{lastRun ? <><StatusDot tone={lastRun.status === "SUCCESS" ? "good" : lastRun.status === "RUNNING" ? "info" : "bad"} />{formatThaiDate(lastRun.startedAt)}</> : "ยังไม่มี"}</p></div>
    </div>

    <SettingsCard title="ฐานข้อมูลบุคลากร" description="ใช้จับคู่ชื่อจาก OCR กับบุคลากรในวิทยาลัย">
      <div className="flex flex-col gap-3 border-b border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 md:flex-row md:items-center lg:px-6">
        <form onSubmit={(event) => { event.preventDefault(); setPage(1); load(query); }} className="relative md:w-[340px]"><MagnifyingGlass size={16} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-[var(--muted)]" /><input value={query} onChange={(event) => setQuery(event.target.value)} aria-label="ค้นหาบุคลากร" placeholder="ค้นหาชื่อ อีเมล หน่วยงาน แล้วกด Enter" className="focus-ring h-10 w-full rounded-full border border-[var(--line-strong)] bg-white pl-10 pr-4 text-[13px] outline-none focus:border-[var(--accent)]" /></form>
        <div role="group" aria-label="กรองแหล่งที่มา" className="flex flex-wrap gap-1.5">{sourceFilters.map(([value, label, count]) => <button key={value} type="button" aria-pressed={source === value} onClick={() => { setSource(value); setPage(1); }} className={`focus-ring h-8 rounded-full border px-3 text-xs ${source === value ? "border-[var(--accent)] bg-[var(--accent)] font-medium text-white" : "border-[var(--line-strong)] bg-white hover:border-[var(--accent)] hover:text-[var(--accent)]"}`}>{label} <span className={source === value ? "" : "text-[var(--muted)]"}>{count}</span></button>)}</div>
      </div>
      <div className="overflow-x-auto"><table className="data-grid w-full min-w-[820px] text-left text-sm"><thead><tr><th className="px-6 py-2.5">ผู้ใช้งาน</th><th className="px-4 py-2.5">ตำแหน่ง</th><th className="px-4 py-2.5">หน่วยงาน</th><th className="px-4 py-2.5">แหล่งที่มา</th><th className="px-6 py-2.5">พบล่าสุด</th></tr></thead>
        <tbody>{visible.map((user) => {
          const name = user.fullName || `${user.firstName} ${user.lastName}`.trim() || user.username || "ไม่ระบุชื่อ";
          return <tr key={user.id} className="hover:bg-[var(--brand-blue-pale)]"><td className="px-6 py-3"><div className="flex items-center gap-3"><span aria-hidden="true" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--line-soft)] text-xs font-semibold text-[var(--navy)]">{initials(name)}</span><div className="min-w-0"><p className="truncate font-medium">{name}</p><p className="truncate text-xs font-light text-[var(--muted)]">{user.email || "ไม่ระบุอีเมล"}</p></div></div></td><td className="px-4 py-3">{user.positionTitle || user.jobTitle || "-"}</td><td className="px-4 py-3 text-[var(--muted)]">{user.department || user.faculty || "-"}</td><td className="px-4 py-3"><span className="inline-flex items-center gap-1.5 text-xs text-[var(--muted)]"><StatusDot tone={user.source === "MANUAL" ? "warn" : "info"} />{user.source === "MANUAL" ? "เจ้าหน้าที่เพิ่ม" : "จาก sync"}</span></td><td className="px-6 py-3 text-xs text-[var(--muted)]">{user.lastSeenAt ? formatThaiDate(user.lastSeenAt) : "—"}</td></tr>;
        })}{visible.length === 0 && <tr><td colSpan={5} className="px-6 py-12 text-center text-sm text-[var(--muted)]">ยังไม่มีข้อมูลบุคลากร กด sync หลังตั้งค่า API_KEY</td></tr>}</tbody></table></div>
      <div className="flex items-center justify-between border-t border-[var(--line)] px-5 py-3 text-[13px] font-light text-[var(--muted)] lg:px-6">
        <span>แสดง {visible.length ? (currentPage - 1) * PAGE_SIZE + 1 : 0}–{(currentPage - 1) * PAGE_SIZE + visible.length} จาก {filtered.length.toLocaleString("th-TH")} รายการ</span>
        <div className="flex items-center gap-1.5"><button type="button" aria-label="หน้าก่อนหน้า" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)} className="focus-ring flex h-9 w-9 items-center justify-center rounded-md border border-[var(--line-strong)] bg-white disabled:opacity-40"><CaretLeft size={15} /></button><span className="px-2 text-[var(--ink)]">{currentPage} / {pageCount}</span><button type="button" aria-label="หน้าถัดไป" disabled={currentPage === pageCount} onClick={() => setPage(currentPage + 1)} className="focus-ring flex h-9 w-9 items-center justify-center rounded-md border border-[var(--line-strong)] bg-white disabled:opacity-40"><CaretRight size={15} /></button></div>
      </div>
    </SettingsCard>

    <SettingsCard title="สิทธิ์ตามบทบาท" description="บทบาทส่งมาจาก SSO ของมหาวิทยาลัย ระบบตรวจสิทธิ์ทุกครั้งที่มีการแก้ไขข้อมูล">
      <div className="overflow-x-auto"><table className="data-grid w-full min-w-[640px] text-left text-sm"><thead><tr><th className="px-6 py-2.5">การทำงาน</th>{ROLES.map((role) => <th key={role} className="px-3 py-2.5 text-center">{roleLabel(role)}<br /><span className="text-[11px] font-light">{role}</span></th>)}</tr></thead>
        <tbody>{PERMISSIONS.map((permission) => <tr key={permission.key}><td className="px-6 py-3">{permission.label}</td>{ROLES.map((role) => <td key={role} className="px-3 py-3 text-center">{permission.roles.includes(role) ? <Check size={18} weight="bold" aria-label="มีสิทธิ์" className="mx-auto text-[var(--success)]" /> : <span aria-label="ไม่มีสิทธิ์" className="text-[#c9d1d9]">—</span>}</td>)}</tr>)}</tbody></table></div>
    </SettingsCard>
  </SettingsFrame>;
}
