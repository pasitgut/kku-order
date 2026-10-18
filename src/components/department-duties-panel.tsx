"use client";

import { useState } from "react";
import { PencilSimple, Plus, X } from "@phosphor-icons/react";
import { Appointment } from "@/lib/api";
import { addDepartmentExtra, departmentDuties, removeDepartmentExtra, setDepartmentDuty } from "@/lib/department-duties";

type Props = {
  appointments: Appointment[];
  editable: boolean;
  onChange: (update: (rows: Appointment[]) => Appointment[]) => void;
};

const textareaClass = "focus-ring w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6 outline-none focus:border-[var(--accent)]";

// แท็บหน้าที่รับผิดชอบ: รายการฝ่ายทางซ้าย เลือกแล้วเห็นหน้าที่ตามคำสั่ง แก้ข้อความหรือเขียนเพิ่มได้
export function DepartmentDutiesPanel({ appointments, editable, onChange }: Props) {
  const departments = departmentDuties(appointments);
  const [picked, setPicked] = useState(departments[0]?.name ?? "");
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [extra, setExtra] = useState("");
  const current = departments.find((item) => item.name === picked) ?? departments[0];
  // ข้อความที่แก้ค้างใช้ได้เฉพาะฝ่ายที่เลือกไว้จริงและเอกสารยังแก้ได้ ฝ่ายหายหรือเอกสารถูกยืนยันระหว่างนั้นจะไม่ไปลงฝ่ายอื่น
  const editingNow = editing && editable && current?.name === picked;

  if (!current) return <p className="p-6 text-sm text-[var(--muted)]">ยังไม่มีรายชื่อในเอกสารนี้</p>;

  const choose = (name: string) => { setPicked(name); setEditing(false); setExtra(""); };
  const startEdit = () => { setDraft(current.duty); setEditing(true); };
  const saveEdit = () => { if (editingNow) onChange((rows) => setDepartmentDuty(rows, current.name, draft)); setEditing(false); };
  const addExtra = () => { if (editable && current.name === picked) onChange((rows) => addDepartmentExtra(rows, current.name, extra)); setExtra(""); };
  const shownMembers = current.members.slice(0, 3).join(", ") + (current.count > 3 ? ` และอีก ${current.count - 3} คน` : "");

  return <div className="grid flex-1 md:grid-cols-[300px_minmax(0,1fr)]">
    <ul aria-label="ฝ่ายในคำสั่ง" className="flex flex-col gap-1 border-b border-[var(--line)] bg-[var(--surface-muted)] p-3 md:border-b-0 md:border-r">
      {departments.map((item) => {
        const selected = item.name === current.name;
        const status = item.duty ? (item.extras.length ? `ตามคำสั่ง · เพิ่มเอง ${item.extras.length}` : "มีหน้าที่ตามคำสั่ง") : item.extras.length ? `เพิ่มเอง ${item.extras.length}` : "ยังไม่มีหน้าที่";
        return <li key={item.name}>
          <button type="button" aria-pressed={selected} onClick={() => choose(item.name)} className={`focus-ring flex min-h-14 w-full items-center gap-3 rounded-lg border px-3.5 py-2.5 text-left ${selected ? "border-[var(--accent)] bg-white shadow-sm" : "border-transparent hover:bg-white"}`}>
            <span className="min-w-0 flex-1">
              <span className="block break-words text-sm font-medium text-[var(--navy)]">{item.name}</span>
              <span className="mt-0.5 block text-xs text-[var(--muted)]">{item.count.toLocaleString("th-TH")} คน · {status}</span>
            </span>
            <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${item.duty || item.extras.length ? "bg-[var(--success)]" : "bg-[var(--warning-dot)]"}`} />
          </button>
        </li>;
      })}
    </ul>

    <div className="flex min-w-0 flex-col gap-5 p-5 lg:p-7">
      <div>
        <h3 className="break-words text-lg font-semibold text-[var(--navy)]">{current.name}</h3>
        <p className="mt-1 text-[13px] text-[var(--muted)]">{current.count.toLocaleString("th-TH")} คน · {shownMembers}</p>
      </div>

      {editingNow ? <div className="space-y-2">
        <label htmlFor="department-duty" className="block text-[13px] font-medium text-[var(--navy)]">หน้าที่ตามคำสั่ง</label>
        <textarea id="department-duty" autoFocus rows={4} value={draft} onChange={(event) => setDraft(event.target.value)} className={textareaClass} />
        <div className="flex justify-end gap-2">
          <button type="button" onClick={() => setEditing(false)} className="focus-ring h-10 rounded-md border border-[var(--line-strong)] px-4 text-sm">ยกเลิก</button>
          <button type="button" onClick={saveEdit} className="focus-ring h-10 rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)]">ใช้ข้อความนี้</button>
        </div>
      </div> : current.duty ? <div className="rounded-lg border border-[var(--line)] p-4">
        <div className="mb-2 flex items-center gap-3">
          <span className="text-xs font-medium text-[var(--muted)]">ตามคำสั่ง{current.pageNo ? ` · หน้า ${current.pageNo}` : ""}</span>
          {editable && <button type="button" onClick={startEdit} className="focus-ring ml-auto inline-flex min-h-9 items-center gap-1 rounded px-1 text-[13px] text-[var(--accent)]"><PencilSimple size={14} />แก้ข้อความ</button>}
        </div>
        <p className="whitespace-pre-line break-words text-[15px] leading-7">{current.duty}</p>
      </div> : <div className="rounded-lg border border-dashed border-[#e5a066] bg-[#fff6ee] p-4 text-sm leading-6 text-[#9a4a0c]">
        {current.extras.length ? "คำสั่งไม่ได้ระบุหน้าที่ของฝ่ายนี้ รายงานใช้หน้าที่ที่เขียนเพิ่มด้านล่าง" : "คำสั่งไม่ได้ระบุหน้าที่ของฝ่ายนี้ รายงานจะใช้บทบาทแทน เช่น “เป็นกรรมการ”"}
        {editable && <button type="button" onClick={startEdit} className="focus-ring ml-1 underline underline-offset-4">พิมพ์หน้าที่ตามคำสั่ง</button>}
      </div>}

      {current.extras.length > 0 && <ul className="space-y-2">
        {current.extras.map((line, index) => <li key={`${line}-${index}`} className="flex items-start gap-2.5 text-[15px] leading-7">
          <span className="mt-1.5 shrink-0 rounded-full bg-[var(--accent-soft)] px-2 text-[11px] font-medium leading-5 text-[var(--accent-strong)]">เพิ่มเอง</span>
          <span className="min-w-0 flex-1 break-words">{line}</span>
          {editable && <button type="button" aria-label={`ลบหน้าที่ ${line}`} onClick={() => onChange((rows) => removeDepartmentExtra(rows, current.name, index))} className="focus-ring flex h-8 w-8 shrink-0 items-center justify-center rounded text-[var(--muted)] hover:bg-[var(--danger-soft)] hover:text-[var(--danger)]"><X size={14} /></button>}
        </li>)}
      </ul>}

      {editable && <div className="space-y-2">
        <label htmlFor="department-extra" className="block text-[13px] font-medium text-[var(--navy)]">เขียนหน้าที่เพิ่ม</label>
        <textarea id="department-extra" rows={3} value={extra} onChange={(event) => setExtra(event.target.value)} placeholder="เช่น จัดทำรายงานสรุปผลการจัดงาน" className={textareaClass} />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-xs text-[var(--muted)]">เพิ่มให้ทุกคนในฝ่ายนี้ แล้วกดบันทึกฉบับร่าง รายงานใช้ข้อความนี้ในคอลัมน์ ภาระ/หน้าที่</p>
          <button type="button" disabled={!extra.trim()} onClick={addExtra} className="focus-ring inline-flex h-10 items-center gap-1.5 rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:opacity-50"><Plus size={15} weight="bold" />เพิ่มหน้าที่</button>
        </div>
      </div>}
    </div>
  </div>;
}
