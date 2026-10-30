"use client";

import { Fragment, useEffect, useRef, useState } from "react";
import { CaretLeft, CaretRight, CheckCircle, MagnifyingGlass, Plus, SpinnerGap, X } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { TeacherProfileEditor } from "@/components/teacher-profile-editor";
import { useToast } from "@/components/toast";
import { ApiError, DirectoryUser, VacantPosition, createDirectoryUser, getDirectoryUsers, getDirectoryUsersPage, getVacantPositions, setPositionHolder } from "@/lib/api";
import { can } from "@/lib/permissions";
import { initials } from "@/lib/person";
import { TeacherDraft, linkedRowsNote, teacherPayload } from "@/lib/position-holder";

const emptyDraft: TeacherDraft = { prefix: "", firstName: "", lastName: "", positionTitle: "", department: "", email: "", phone: "", managePosition: "", managePositionSince: "" };
type ManagerPositionDraft = { positionTitle: string; startsOn: string; endsOn: string };
const inputClass = "focus-ring h-11 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm outline-none focus:border-[var(--accent)]";
const buttonClass = "focus-ring inline-flex min-h-11 items-center justify-center gap-2 rounded-md px-4 text-sm font-medium disabled:opacity-50";

export default function TeachersSettingsPage() {
  const { me, status } = useMe();
  const toast = useToast();
  const [draft, setDraft] = useState(emptyDraft);
  const [managerPositions, setManagerPositions] = useState<ManagerPositionDraft[]>([]);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);
  const firstNameRef = useRef<HTMLInputElement>(null);
  const [formError, setFormError] = useState("");
  const [success, setSuccess] = useState("");
  const [conflictName, setConflictName] = useState("");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [source, setSource] = useState("all");
  const [page, setPage] = useState(1);
  const [revision, setRevision] = useState(0);
  const [list, setList] = useState<{ data: DirectoryUser[]; pagination: { total: number; page: number; limit: number } } | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [vacancies, setVacancies] = useState<VacantPosition[] | null>(null);
  const [vacancyRevision, setVacancyRevision] = useState(0);
  const [editingUserId, setEditingUserId] = useState<number | null>(null);
  const canCreate = can(me?.role ?? "", "createPerson");

  useEffect(() => {
    if (success && !saving) firstNameRef.current?.focus();
  }, [success, saving]);

  useEffect(() => {
    let active = true;
    getVacantPositions().then((response) => { if (active) setVacancies(response.data); }).catch(() => { if (active) setVacancies([]); });
    return () => { active = false; };
  }, [vacancyRevision]);

  useEffect(() => {
    let active = true;
    getDirectoryUsersPage(search, source, page).then((response) => {
      if (active) { setList(response); setLoadError(""); setLoading(false); }
    }).catch((reason: unknown) => {
      if (active) { setLoadError(reason instanceof Error ? reason.message : "โหลดรายชื่อไม่สำเร็จ"); setLoading(false); }
    });
    return () => { active = false; };
  }, [search, source, page, revision]);

  const refreshList = () => { setLoading(true); setRevision((current) => current + 1); };
  const searchFor = (value: string) => {
    setQuery(value); setSearch(value.trim()); setSource("all"); setPage(1); refreshList();
  };

  const updateManagerPosition = (index: number, key: keyof ManagerPositionDraft, value: string) => {
    setManagerPositions((current) => current.map((position, row) => row === index ? { ...position, [key]: value } : position));
  };

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canCreate || savingRef.current) return;
    const profilePayload = teacherPayload(draft);
    const positionPayload = managerPositions.map((position) => ({
      positionTitle: position.positionTitle.trim(),
      startsOn: position.startsOn,
      ...(position.endsOn ? { endsOn: position.endsOn } : {}),
    }));
    const payload = { ...profilePayload, ...(positionPayload.length ? { positions: positionPayload } : {}) };
    setFormError(""); setConflictName(""); setSuccess("");
    if (!payload.firstName || !payload.lastName) {
      setFormError("กรุณาระบุชื่อและนามสกุล โดยไม่ใช้ช่องว่างเพียงอย่างเดียว");
      return;
    }
    if (managerPositions.some((position) => !position.positionTitle.trim() || !position.startsOn || (position.endsOn && position.endsOn < position.startsOn))) {
      setFormError("กรุณาระบุตำแหน่งและวันที่เริ่มให้ครบ และตรวจวันที่สิ้นสุด");
      return;
    }
    savingRef.current = true; setSaving(true);
    try {
      const { data, linked } = await createDirectoryUser(payload);
      const name = data.fullName || `${data.firstName} ${data.lastName}`;
      setSuccess(`เพิ่ม ${name} แล้ว สามารถค้นหาและเลือกใช้ในเอกสารได้ทันที${linkedRowsNote(linked)}`);
      toast(`เพิ่มข้อมูล ${name} แล้ว`);
      setDraft(emptyDraft); setManagerPositions([]); setQuery(""); setSearch(""); setSource("MANUAL"); setPage(1); refreshList();
      if (payload.positions?.length || payload.managePosition) setVacancyRevision((current) => current + 1);
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : "ไม่สามารถเพิ่มข้อมูลอาจารย์ได้ กรุณาลองอีกครั้ง");
      if (reason instanceof ApiError && reason.status === 409 && typeof reason.details.directoryUserId === "number") {
        setConflictName(`${payload.firstName} ${payload.lastName}`);
      }
    } finally {
      savingRef.current = false; setSaving(false);
    }
  };

  const currentPage = list?.pagination.page ?? 1;
  const total = list?.pagination.total ?? 0;
  const limit = list?.pagination.limit ?? 25;
  const pageCount = Math.max(1, Math.ceil(total / limit));

  return <SettingsFrame note="เพิ่มรายชื่ออาจารย์และบุคลากรไว้ล่วงหน้า เพื่อเลือกใช้ตอนตรวจเอกสารและจับคู่ชื่อจาก OCR ได้ โดยไม่ต้องเพิ่มเอกสารก่อน">
    <div>
      <h2 className="text-xl font-semibold text-[var(--navy)]">ข้อมูลอาจารย์และบุคลากร</h2>
      <p className="mt-1 text-sm font-light leading-6 text-[var(--muted)]">เตรียมรายชื่อไว้ก่อนเริ่มงานเอกสาร รวมถึงบุคลากรจากหน่วยงานภายนอก</p>
    </div>

    <div className={canCreate ? "grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_340px]" : ""}>
    {canCreate && <SettingsCard id="add-teacher" title="เพิ่มข้อมูลอาจารย์" description="ระบุชื่อและนามสกุล ส่วนข้อมูลอื่นสามารถเว้นว่างได้">
      <form onSubmit={submit} className="space-y-5 p-5 lg:p-6" aria-busy={saving}>
        {formError && <div role="alert" className="rounded-md bg-[var(--danger-soft)] px-4 py-3 text-sm text-[var(--danger)]">
          <p>{formError}</p>
          {conflictName && <button type="button" onClick={() => searchFor(conflictName)} className="focus-ring mt-2 min-h-10 underline underline-offset-4">ดูรายชื่อที่มีอยู่แล้ว</button>}
        </div>}
        {success && <p role="status" className="flex items-start gap-2 rounded-md bg-[var(--success-soft)] px-4 py-3 text-sm leading-6 text-[var(--success)]"><CheckCircle size={20} className="mt-0.5 shrink-0" />{success}</p>}
        <fieldset disabled={saving} className="grid gap-4 sm:grid-cols-2">
          <label className="space-y-1.5 text-sm sm:col-span-2"><span className="block">คำนำหน้า</span><input name="prefix" value={draft.prefix} onChange={(event) => setDraft({ ...draft, prefix: event.target.value })} maxLength={50} placeholder="เช่น ผศ.ดร. หรือ อ." autoComplete="honorific-prefix" className={`${inputClass} sm:max-w-64`} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">ชื่อ <span className="text-[var(--danger)]">*</span></span><input ref={firstNameRef} name="firstName" required value={draft.firstName} onChange={(event) => setDraft({ ...draft, firstName: event.target.value })} maxLength={255} autoComplete="given-name" className={inputClass} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">นามสกุล <span className="text-[var(--danger)]">*</span></span><input name="lastName" required value={draft.lastName} onChange={(event) => setDraft({ ...draft, lastName: event.target.value })} maxLength={255} autoComplete="family-name" className={inputClass} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">ตำแหน่ง</span><input name="positionTitle" value={draft.positionTitle} onChange={(event) => setDraft({ ...draft, positionTitle: event.target.value })} maxLength={255} placeholder="เช่น อาจารย์" autoComplete="organization-title" className={inputClass} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">หน่วยงาน / สังกัด</span><input name="department" value={draft.department} onChange={(event) => setDraft({ ...draft, department: event.target.value })} maxLength={255} placeholder="เช่น วิทยาลัยการคอมพิวเตอร์" autoComplete="organization" className={inputClass} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">อีเมล</span><input name="email" type="email" value={draft.email} onChange={(event) => setDraft({ ...draft, email: event.target.value })} maxLength={255} placeholder="name@example.com" autoComplete="email" className={inputClass} /></label>
          <label className="space-y-1.5 text-sm"><span className="block">เบอร์โทรศัพท์</span><input name="phone" type="tel" value={draft.phone} onChange={(event) => setDraft({ ...draft, phone: event.target.value })} maxLength={20} autoComplete="tel" className={inputClass} /></label>
          <div className="space-y-3 rounded-lg border border-[var(--accent)] bg-[var(--accent-soft)] p-4 sm:col-span-2">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div><p className="text-sm font-medium">ตำแหน่งบริหารตามช่วงเวลา (ไม่บังคับ)</p><p className="mt-1 text-xs font-light leading-5 text-[var(--muted)]">เพิ่มได้หลายตำแหน่ง เช่น คณบดีหรือประธานหลักสูตร ระบบใช้วันที่ออกคำสั่งเลือกผู้ดำรงตำแหน่ง</p></div>
              <button type="button" disabled={managerPositions.length >= 20} onClick={() => setManagerPositions((current) => [...current, { positionTitle: "", startsOn: "", endsOn: "" }])} className="focus-ring inline-flex min-h-10 items-center gap-1.5 rounded-md border border-[var(--line-strong)] bg-white px-3 text-xs font-medium text-[var(--accent)] hover:bg-[var(--canvas)] disabled:cursor-not-allowed disabled:opacity-50"><Plus size={15} weight="bold" />เพิ่มตำแหน่ง</button>
            </div>
            {managerPositions.map((position, index) => <div key={index} className="grid gap-3 rounded-md border border-[var(--line)] bg-white p-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
              <label className="space-y-1.5 text-xs"><span className="block">ตำแหน่งที่ {index + 1}</span><input required name={`positions.${index}.positionTitle`} list="vacant-position-titles" value={position.positionTitle} onChange={(event) => updateManagerPosition(index, "positionTitle", event.target.value)} maxLength={255} placeholder="เช่น ผู้ช่วยคณบดีฝ่ายวิชาการ" className={inputClass} /></label>
              <label className="space-y-1.5 text-xs"><span className="block">เริ่มดำรงตำแหน่ง</span><input required type="date" name={`positions.${index}.startsOn`} value={position.startsOn} onChange={(event) => updateManagerPosition(index, "startsOn", event.target.value)} className={inputClass} /></label>
              <label className="space-y-1.5 text-xs"><span className="block">สิ้นสุด (เว้นว่างถ้ายังดำรงอยู่)</span><input type="date" name={`positions.${index}.endsOn`} min={position.startsOn || undefined} value={position.endsOn} onChange={(event) => updateManagerPosition(index, "endsOn", event.target.value)} className={inputClass} /></label>
              <button type="button" aria-label={`ลบตำแหน่งที่ ${index + 1}`} onClick={() => setManagerPositions((current) => current.filter((_, row) => row !== index))} className="focus-ring inline-flex h-10 w-10 items-center justify-center rounded-md border border-[var(--line)] text-[var(--muted)] hover:bg-[var(--danger-soft)] hover:text-[var(--danger)]"><X size={17} /></button>
            </div>)}
            <p className="text-xs font-light leading-5 text-[var(--muted)]">เพิ่มได้สูงสุด 20 ช่วง ถ้าเพิ่มช่วงใหม่ของตำแหน่งเดิม ระบบจะปิดช่วงก่อนหน้าก่อนวันเริ่มใหม่หนึ่งวัน และใช้ช่วงเวลานี้ผูกแถว &quot;โดยตำแหน่ง&quot; ในเอกสารที่รอยืนยัน</p>
          </div>
        </fieldset>
        <div className="flex flex-col gap-3 border-t border-[var(--line-soft)] pt-4 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs font-light leading-5 text-[var(--muted)]">รายชื่อนี้ใช้กับเอกสารเท่านั้น ไม่มีการสร้างบัญชีเข้าสู่ระบบ</p>
          <button type="submit" disabled={saving} className={`${buttonClass} shrink-0 bg-[var(--accent)] text-white hover:bg-[var(--accent-strong)]`}>{saving ? <SpinnerGap size={18} className="animate-spin" /> : <Plus size={18} weight="bold" />}{saving ? "กำลังบันทึก..." : "เพิ่มข้อมูลอาจารย์"}</button>
        </div>
      </form>
    </SettingsCard>}
    <VacantPositionsCard vacancies={vacancies} canAssign={canCreate} onAssigned={() => { setVacancyRevision((current) => current + 1); refreshList(); }} />
    <datalist id="vacant-position-titles">{vacancies?.map((item) => <option key={item.positionTitle} value={item.positionTitle} />)}</datalist>
    </div>
    {!canCreate && status === "ready" && <p className="rounded-md border border-[var(--line)] bg-[var(--surface-muted)] px-4 py-3 text-sm leading-6 text-[var(--muted)]">ผู้ดูแลระบบและเจ้าหน้าที่สามารถเพิ่มหรือแก้ไขข้อมูลอาจารย์ได้ คุณสามารถค้นหาและดูรายชื่อด้านล่าง</p>}

    <SettingsCard id="teacher-list" title="รายชื่อในระบบ" description="รายชื่อชุดเดียวกับที่ใช้เลือกบุคคลในเอกสาร เรียงตามรายการที่เพิ่มล่าสุด กดแก้ไขเพื่อปรับข้อมูลและดูประวัติตำแหน่ง">
      <div className="flex flex-col gap-3 border-b border-[var(--line)] bg-[var(--surface-muted)] px-5 py-4 lg:px-6">
        <form onSubmit={(event) => { event.preventDefault(); setSearch(query.trim()); setPage(1); refreshList(); }} role="search" aria-label="ค้นหารายชื่ออาจารย์" className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1"><MagnifyingGlass size={18} aria-hidden="true" className="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--muted)]" /><input value={query} onChange={(event) => setQuery(event.target.value)} aria-label="ค้นหาชื่อ อีเมล หน่วยงาน หรือเบอร์โทร" placeholder="ค้นหาชื่อ อีเมล หน่วยงาน หรือเบอร์โทร" className={`${inputClass} pl-10`} /></div>
          <button type="submit" className={`${buttonClass} border border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--accent-soft)]`}>ค้นหา</button>
        </form>
        <div role="group" aria-label="กรองแหล่งที่มาของรายชื่อ" className="flex flex-wrap gap-2">{[["all", "ทั้งหมด"], ["MANUAL", "เพิ่มเอง"], ["SYNCED", "จาก sync"]].map(([value, label]) => <button key={value} type="button" aria-pressed={source === value} onClick={() => { setSource(value); setPage(1); refreshList(); }} className={`focus-ring min-h-10 rounded-full border px-4 text-xs ${source === value ? "border-[var(--accent)] bg-[var(--accent)] font-medium text-white" : "border-[var(--line-strong)] bg-white hover:border-[var(--accent)]"}`}>{label}</button>)}</div>
      </div>
      <div aria-busy={loading}>
        {loading ? <p role="status" className="flex items-center justify-center gap-2 px-5 py-12 text-sm text-[var(--muted)]"><SpinnerGap size={18} className="animate-spin" />กำลังโหลดรายชื่อ...</p>
          : loadError ? <div role="alert" className="px-5 py-8 text-sm text-[var(--danger)]"><p>{loadError}</p><button type="button" onClick={refreshList} className={`${buttonClass} mt-3 border border-[var(--line-strong)] text-[var(--accent)]`}>ลองใหม่</button></div>
          : <>
            <div className="overflow-x-auto"><table className="data-grid w-full min-w-[760px] text-left text-sm"><caption className="sr-only">รายชื่ออาจารย์และบุคลากรในระบบ</caption>
              <thead><tr><th scope="col" className="px-5 py-3 lg:px-6">ชื่อ–นามสกุล</th><th scope="col" className="px-4 py-3">ตำแหน่ง / หน่วยงาน</th><th scope="col" className="px-4 py-3">ติดต่อ</th><th scope="col" className="px-5 py-3">แหล่งที่มา</th>{canCreate && <th scope="col" className="px-4 py-3">จัดการ</th>}</tr></thead>
              <tbody>{list?.data.map((user) => {
                const name = user.fullName || `${user.prefix} ${user.firstName} ${user.lastName}`.trim();
                return <Fragment key={user.id}>
                  <tr className="hover:bg-[var(--brand-blue-pale)]">
                    <td className="px-5 py-4 lg:px-6"><div className="flex items-center gap-3"><span aria-hidden="true" className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-xs font-medium text-[var(--navy)]">{initials(name)}</span><span className="break-words font-medium">{name}</span></div></td>
                    <td className="px-4 py-4"><p className="break-words">{user.positionTitle || user.jobTitle || "ไม่ระบุตำแหน่ง"}</p><p className="mt-1 break-words text-xs font-light text-[var(--muted)]">{user.department || user.faculty || "ไม่ระบุหน่วยงาน"}</p></td>
                    <td className="px-4 py-4"><p className="break-all text-xs">{user.email || "ไม่ระบุอีเมล"}</p><p className="mt-1 text-xs font-light text-[var(--muted)]">{user.phone || "ไม่ระบุเบอร์โทร"}</p></td>
                    <td className="px-5 py-4"><span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-[var(--muted)]"><StatusDot tone={user.source === "MANUAL" ? "warn" : "info"} />{user.source === "MANUAL" ? "เพิ่มเอง" : "จาก sync"}</span></td>
                    {canCreate && <td className="px-4 py-4"><button type="button" aria-expanded={editingUserId === user.id} onClick={() => setEditingUserId(editingUserId === user.id ? null : user.id)} className="focus-ring min-h-10 rounded-md border border-[var(--line-strong)] bg-white px-3 text-xs font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]">{editingUserId === user.id ? "ปิด" : "แก้ไข / ตำแหน่ง"}</button></td>}
                  </tr>
                  {canCreate && editingUserId === user.id && <tr><td colSpan={5} className="px-3 py-3 sm:px-5"><TeacherProfileEditor user={user} onCancel={() => setEditingUserId(null)} onSaved={() => { setEditingUserId(null); refreshList(); }} /></td></tr>}
                </Fragment>;
              })}{!list?.data.length && <tr><td colSpan={canCreate ? 5 : 4} className="px-5 py-12 text-center leading-7 text-[var(--muted)]">{search || source !== "all" ? "ไม่พบรายชื่อที่ตรงกับการค้นหาหรือตัวกรอง" : "ยังไม่มีรายชื่อในระบบ"}<p className="text-xs font-light">{search || source !== "all" ? "ลองเปลี่ยนคำค้นหรือเลือกแหล่งที่มาเป็นทั้งหมด" : canCreate ? "กรอกแบบฟอร์มด้านบนเพื่อเพิ่มอาจารย์คนแรกได้เลย" : "ติดต่อผู้ดูแลระบบหรือเจ้าหน้าที่เพื่อเพิ่มรายชื่อ"}</p></td></tr>}</tbody>
            </table></div>
            <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[var(--line)] px-5 py-3 text-xs text-[var(--muted)] lg:px-6">
              <p role="status">แสดง {total ? (currentPage - 1) * limit + 1 : 0}–{Math.min(currentPage * limit, total)} จาก {total.toLocaleString("th-TH")} รายการ</p>
              <div className="flex items-center gap-2"><button type="button" aria-label="หน้าก่อนหน้า" disabled={currentPage <= 1} onClick={() => { setPage(currentPage - 1); setLoading(true); }} className={`${buttonClass} border border-[var(--line-strong)] px-3`}><CaretLeft size={16} /></button><span>{currentPage} / {pageCount}</span><button type="button" aria-label="หน้าถัดไป" disabled={currentPage >= pageCount} onClick={() => { setPage(currentPage + 1); setLoading(true); }} className={`${buttonClass} border border-[var(--line-strong)] px-3`}><CaretRight size={16} /></button></div>
            </div>
          </>}
      </div>
    </SettingsCard>
  </SettingsFrame>;
}

// VacantPositionsCard แสดงตำแหน่งที่เอกสารรอยืนยันอ้างถึงแต่ยังไม่มีผู้ดำรง ให้เลือกคนที่มีอยู่แล้วในระบบได้ทันที
function VacantPositionsCard({ vacancies, canAssign, onAssigned }: { vacancies: VacantPosition[] | null; canAssign: boolean; onAssigned: () => void }) {
  const [open, setOpen] = useState("");
  return <SettingsCard id="vacant-positions" title="ตำแหน่งที่ยังไม่มีผู้ดำรง" description="จากเอกสารที่รอยืนยัน กรอกให้ครบครั้งเดียวก่อนเริ่มใช้งานจริง">
    {vacancies === null ? <p role="status" className="flex items-center gap-2 px-5 py-6 text-sm text-[var(--muted)]"><SpinnerGap size={18} className="animate-spin" />กำลังโหลด...</p>
      : !vacancies.length ? <p className="flex items-start gap-2 px-5 py-6 text-sm leading-6 text-[var(--success)]"><CheckCircle size={20} className="mt-0.5 shrink-0" />ทุกตำแหน่งในเอกสารที่รอยืนยันมีผู้ดำรงแล้ว</p>
      : <ul className="divide-y divide-[var(--line-soft)]">{vacancies.map((item) => <li key={item.positionTitle} className="px-5 py-3">
        <div className="flex items-start justify-between gap-3">
          <p className="min-w-0 break-words text-sm">{item.positionTitle}</p>
          <span className="shrink-0 whitespace-nowrap text-xs text-[var(--muted)]">{item.documents.toLocaleString("th-TH")} คำสั่ง</span>
        </div>
        {canAssign && open !== item.positionTitle && <button type="button" onClick={() => setOpen(item.positionTitle)} className="focus-ring mt-1 min-h-9 text-xs font-medium text-[var(--accent)] underline underline-offset-4">กำหนดผู้ดำรงตำแหน่ง</button>}
        {canAssign && open === item.positionTitle && <AssignHolderForm positionTitle={item.positionTitle} onCancel={() => setOpen("")} onAssigned={() => { setOpen(""); onAssigned(); }} />}
      </li>)}</ul>}
    <p className="border-t border-[var(--line)] px-5 py-3 text-xs font-light leading-5 text-[var(--muted)]">บทบาทในคณะกรรมการ เช่น ประธานกรรมการ ไม่อยู่ในรายการนี้ ให้เลือกคนในหน้าตรวจเอกสาร</p>
  </SettingsCard>;
}

function AssignHolderForm({ positionTitle, onCancel, onAssigned }: { positionTitle: string; onCancel: () => void; onAssigned: () => void }) {
  const toast = useToast();
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<DirectoryUser[]>([]);
  const [personId, setPersonId] = useState(0);
  const [startsOn, setStartsOn] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    const text = query.trim();
    if (!text) return;
    let active = true;
    const timer = setTimeout(() => {
      getDirectoryUsers(text).then((response) => { if (active) setMatches(response.data.slice(0, 6)); }).catch(() => { if (active) setMatches([]); });
    }, 250);
    return () => { active = false; clearTimeout(timer); };
  }, [query]);

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!personId || saving) { setError(personId ? "" : "กรุณาเลือกผู้ดำรงตำแหน่ง"); return; }
    setSaving(true); setError("");
    try {
      const { linked } = await setPositionHolder({ positionTitle, directoryUserId: personId, startsOn: startsOn.trim() });
      toast(`บันทึกผู้ดำรงตำแหน่ง ${positionTitle} แล้ว${linkedRowsNote(linked)}`);
      onAssigned();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "บันทึกผู้ดำรงตำแหน่งไม่สำเร็จ");
    } finally {
      setSaving(false);
    }
  };

  const shown = query.trim() ? matches : [];
  return <form onSubmit={save} className="mt-3 space-y-3 rounded-lg border border-[var(--line-strong)] bg-[var(--surface-muted)] p-3" aria-busy={saving}>
    {error && <p role="alert" className="rounded-md bg-[var(--danger-soft)] px-3 py-2 text-xs text-[var(--danger)]">{error}</p>}
    <label className="block space-y-1.5 text-xs"><span className="block">ค้นหาผู้ดำรงตำแหน่ง</span><input value={query} onChange={(event) => { setQuery(event.target.value); setPersonId(0); }} placeholder="พิมพ์ชื่อหรือนามสกุล" autoFocus className={`${inputClass} h-10`} /></label>
    {shown.length > 0 && <fieldset className="space-y-1"><legend className="sr-only">เลือกบุคคล</legend>{shown.map((user) => {
      const name = user.fullName || `${user.prefix} ${user.firstName} ${user.lastName}`.trim();
      return <label key={user.id} className={`flex min-h-10 cursor-pointer items-center gap-2 rounded-md border px-3 text-xs ${personId === user.id ? "border-[var(--accent)] bg-white" : "border-transparent hover:bg-white"}`}><input type="radio" name="holder" checked={personId === user.id} onChange={() => setPersonId(user.id)} /><span className="min-w-0 break-words">{name}<span className="block font-light text-[var(--muted)]">{user.positionTitle || user.jobTitle || ""}</span></span></label>;
    })}</fieldset>}
    {query.trim() && !shown.length && <p className="text-xs text-[var(--muted)]">ไม่พบรายชื่อ เพิ่มคนใหม่ได้จากแบบฟอร์ม &quot;เพิ่มข้อมูลอาจารย์&quot;</p>}
    <label className="block space-y-1.5 text-xs"><span className="block">เริ่มดำรงตำแหน่ง</span><input required type="date" value={startsOn} onChange={(event) => setStartsOn(event.target.value)} className={`${inputClass} h-10`} /></label>
    <div className="flex justify-end gap-2">
      <button type="button" onClick={onCancel} className={`${buttonClass} min-h-10 border border-[var(--line-strong)] bg-white px-3`}>ยกเลิก</button>
      <button type="submit" disabled={saving} className={`${buttonClass} min-h-10 bg-[var(--accent)] px-3 text-white hover:bg-[var(--accent-strong)]`}>{saving && <SpinnerGap size={16} className="animate-spin" />}บันทึก</button>
    </div>
  </form>;
}
