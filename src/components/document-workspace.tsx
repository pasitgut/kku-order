"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, CaretLeft, CaretRight, CheckCircle, FloppyDisk, Info, MapPin, MagnifyingGlass, Trash, WarningCircle } from "@phosphor-icons/react";
import { AppShell, StatusBadge } from "@/components/app-shell";
import { ApiDocument, Appointment, DocumentRevision, confirmDocument, deleteDocument, formatFileSize, formatThaiDate, getDocumentPageImageUrl, getDocumentRevisions, statusLabel, updateDocument } from "@/lib/api";

type PanelView = "members" | "metadata" | "duties";
type MemberFilter = "all" | "warning" | "verified" | "pending";

const metadataKeys = ["order_type", "order_no", "order_year_be", "committee_name", "issued_date", "effective_date", "expiry_date", "signer_name", "signer_position"];

function matchState(row: Appointment) {
  if (row.nameMatchMethod === "EXACT" && (row.nameMatchScore ?? 0) >= 0.9) return { label: "พบรายชื่อในระบบ", tone: "good" };
  if (row.nameMatchMethod === "FUZZY") return { label: `ชื่อใกล้เคียงในระบบ ${Math.round((row.nameMatchScore ?? 0) * 100)}%`, tone: "warning" };
  return { label: "ไม่พบรายชื่อในระบบ", tone: "danger" };
}

export function DocumentWorkspace({ document: initialDocument }: { document: ApiDocument }) {
  const router = useRouter();
  const [document, setDocument] = useState(initialDocument);
  const [fields, setFields] = useState(initialDocument.fields ?? []);
  const [appointments, setAppointments] = useState(initialDocument.appointments ?? []);
  const [revisions, setRevisions] = useState<DocumentRevision[]>([]);
  const [selectedPage, setSelectedPage] = useState(1);
  const [selectedAppointmentId, setSelectedAppointmentId] = useState<number | null>(null);
  const [panel, setPanel] = useState<PanelView>("members");
  const [memberFilter, setMemberFilter] = useState<MemberFilter>("all");
  const [memberQuery, setMemberQuery] = useState("");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [deleteArmed, setDeleteArmed] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const selectedOCRPage = document.ocrPages?.find((page) => page.pageNo === selectedPage);
  const pageWidth = selectedOCRPage?.imageWidth || 1;
  const pageHeight = selectedOCRPage?.imageHeight || 1;
  const metadataFields = fields.filter((field) => metadataKeys.includes(field.fieldKey));
  const lowConfidenceCount = useMemo(() => fields.filter((field) => field.confidence < 0.9 || !field.value).length + appointments.filter((item) => item.confidence < 0.9 || item.nameMatchMethod !== "EXACT").length, [fields, appointments]);

  const filteredAppointments = useMemo(() => {
    const query = memberQuery.trim().toLowerCase();
    return appointments.filter((item) => {
      const matchesQuery = !query || [item.fullName, item.position, item.committeeRole, item.department].join(" ").toLowerCase().includes(query);
      const state = matchState(item);
      const matchesStatus = memberFilter === "all" || (memberFilter === "verified" && item.verified) || (memberFilter === "pending" && !item.verified && state.tone === "good") || (memberFilter === "warning" && state.tone !== "good");
      return matchesQuery && matchesStatus;
    });
  }, [appointments, memberFilter, memberQuery]);

  const departmentGroups = useMemo(() => {
    const groups = new Map<string, Appointment[]>();
    for (const item of filteredAppointments) {
      const department = item.department?.trim() || "ไม่ระบุฝ่าย";
      groups.set(department, [...(groups.get(department) ?? []), item]);
    }
    return [...groups.entries()];
  }, [filteredAppointments]);

  const parseBox = (raw: string) => {
    try {
      const box = JSON.parse(raw) as unknown;
      if (!Array.isArray(box) || box.length < 4) return null;
      const [left, top, width, height] = box.map(Number);
      if (![left, top, width, height].every(Number.isFinite)) return null;
      return { left: `${Math.max(0, left / pageWidth) * 100}%`, top: `${Math.max(0, top / pageHeight) * 100}%`, width: `${Math.max(0, width) / pageWidth * 100}%`, height: `${Math.max(0, height) / pageHeight * 100}%` };
    } catch {
      return null;
    }
  };

  useEffect(() => {
    getDocumentRevisions(document.id).then((response) => setRevisions(response.data)).catch(() => setRevisions([]));
  }, [document.id]);

  const updateField = (id: number, value: string) => setFields((current) => current.map((field) => field.id === id ? { ...field, value } : field));
  const updateAppointment = (id: number, key: "fullName" | "position" | "committeeRole" | "verified", value: string | boolean) => setAppointments((current) => current.map((item) => item.id === id ? { ...item, [key]: value } : item));
  const locateAppointment = (id: number, pageNo: number) => { setSelectedAppointmentId(id); setSelectedPage(Math.max(1, pageNo || 1)); };
  const buildPayload = () => ({
    responsibilities: document.responsibilities,
    additionalReferences: document.additionalReferences,
    fields: fields.map((field) => ({ id: field.id, fieldKey: field.fieldKey, fieldLabel: field.fieldLabel, value: field.value, verified: field.verified })),
    appointments: appointments.map((item) => ({ id: item.id, fullName: item.fullName, position: item.position, department: item.department, committeeRole: item.committeeRole, responsibilities: item.responsibilities, confidence: item.confidence, pageNo: item.pageNo, boundingBox: item.boundingBox, verified: item.verified })),
  });

  const saveDraft = async (confirm = false) => {
    setSaving(true); setError(""); setMessage("");
    try {
      const response = await updateDocument(document.id, { ...buildPayload(), reviewNotes: document.reviewNotes });
      setDocument(response.data); setFields(response.data.fields ?? fields); setAppointments(response.data.appointments ?? appointments);
      if (confirm) {
        await confirmDocument(document.id);
        setDocument((current) => ({ ...current, status: "CONFIRMED", confirmedAt: new Date().toISOString(), confirmedBy: "local-user" }));
        setMessage("บันทึกและยืนยันข้อมูลเรียบร้อยแล้ว");
      } else setMessage("บันทึกฉบับร่างลงฐานข้อมูลแล้ว");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "ไม่สามารถบันทึกข้อมูลได้");
    } finally {
      setSaving(false);
    }
  };

  const removeDocument = async () => {
    if (!deleteArmed) {
      setDeleteArmed(true);
      return;
    }
    setDeleting(true); setError("");
    try {
      await deleteDocument(document.id);
      router.replace("/documents");
      router.refresh();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "ไม่สามารถลบข้อมูลได้");
      setDeleteArmed(false);
    } finally {
      setDeleting(false);
    }
  };

  const tabClass = (value: PanelView) => `border-b-2 px-4 py-3 text-sm font-bold transition ${panel === value ? "border-[var(--accent)] text-[var(--accent)]" : "border-transparent text-[var(--muted)] hover:text-[var(--ink)]"}`;
  const filterClass = (value: MemberFilter) => `rounded-full border px-3 py-1.5 ${memberFilter === value ? "border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--accent)]" : "border-[var(--line)] text-[var(--muted)] hover:border-[var(--accent)]"}`;

  return <AppShell><div className="space-y-6">
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div className="flex items-center gap-3"><Link href="/documents" aria-label="กลับไปยังเอกสารทั้งหมด" className="focus-ring rounded-lg border border-[var(--line)] p-2 text-[var(--muted)] hover:border-[var(--accent)] hover:text-[var(--accent)]"><ArrowLeft size={18} /></Link><div><p className="text-xs font-semibold text-[var(--muted)]">เอกสารทั้งหมด / ตรวจสอบข้อมูลจริง</p><h1 className="mt-1 max-w-[700px] text-xl font-bold tracking-tight text-[var(--ink)] sm:text-2xl">{document.title || document.originalFilename}</h1></div></div><div className="flex flex-wrap items-center gap-2"><StatusBadge status={statusLabel(document.status)} /><span className="text-xs text-[var(--muted)]">นำเข้า {formatThaiDate(document.createdAt)}</span><button type="button" onClick={removeDocument} disabled={deleting} className={`focus-ring inline-flex h-9 items-center gap-1.5 rounded-lg border px-3 text-xs font-bold transition ${deleteArmed ? "border-[var(--danger)] bg-[var(--danger)] text-white" : "border-[var(--danger)]/40 text-[var(--danger)] hover:bg-[var(--danger-soft)]"}`}><Trash size={15} />{deleting ? "กำลังลบ..." : deleteArmed ? "ยืนยันการลบ" : "ลบข้อมูลทดสอบ"}</button>{deleteArmed && !deleting && <button type="button" onClick={() => setDeleteArmed(false)} className="focus-ring rounded-lg px-2 text-xs font-semibold text-[var(--muted)] hover:text-[var(--ink)]">ยกเลิก</button>}</div></div>
    {error && <div className="rounded-lg border border-[var(--danger-soft)] bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}{message && <div className="rounded-lg border border-[var(--success-soft)] bg-[var(--success-soft)] p-4 text-sm font-medium text-[var(--success)]">{message}</div>}
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1.05fr)_minmax(560px,1fr)]">
      <section className="flex min-h-[720px] flex-col overflow-hidden rounded-xl border border-[var(--line)] bg-[#e9eef3]"><div className="flex items-center justify-between border-b border-[#d5dee7] bg-white px-4 py-3"><div><p className="text-sm font-bold text-[var(--ink)]">ต้นฉบับเอกสาร</p><p className="mt-0.5 text-xs text-[var(--muted)]">{document.pageCount || "-"} หน้า · {formatFileSize(document.fileSizeBytes)} · {document.ocrProvider || "นำเข้าข้อมูลโดยตรง"}</p></div><div className="flex items-center gap-1"><span className="px-2 text-xs font-semibold text-[var(--muted)]">หน้า {selectedPage}</span><button type="button" onClick={() => setSelectedPage((page) => Math.max(1, page - 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[#f0f4f7]" aria-label="หน้าก่อนหน้า"><CaretLeft size={17} /></button><button type="button" onClick={() => setSelectedPage((page) => Math.min(document.pageCount || 1, page + 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[#f0f4f7]" aria-label="หน้าถัดไป"><CaretRight size={17} /></button></div></div><div className="shell-scrollbar flex flex-1 items-start justify-center overflow-auto p-6">{document.sourceType.toLowerCase() === "pdf" ? <div className="relative w-full max-w-[560px] overflow-hidden bg-white shadow-[0_8px_28px_rgba(17,40,62,0.14)]"><img src={getDocumentPageImageUrl(document.id, selectedPage)} alt={`หน้า ${selectedPage} ของ ${document.title}`} className="block h-auto w-full" /><div className="pointer-events-none absolute inset-0">{(selectedOCRPage?.lines ?? []).map((line) => { const box = parseBox(line.boundingBox); return box ? <span key={line.id} title={line.text} className="absolute border border-sky-400/50 bg-sky-300/5" style={box} /> : null; })}{appointments.filter((item) => item.pageNo === selectedPage).map((item) => { const box = parseBox(item.boundingBox); return box ? <span key={`appointment-${item.id}`} title={item.fullName} className={`absolute border-2 ${selectedAppointmentId === item.id ? "z-10 border-rose-500 bg-rose-300/35" : "border-amber-500 bg-amber-300/20"}`} style={box} /> : null; })}{fields.filter((field) => field.pageNo === selectedPage).map((field) => { const box = parseBox(field.boundingBox); return box ? <span key={`field-${field.id}`} title={field.fieldLabel} className="absolute border-2 border-emerald-500 bg-emerald-300/20" style={box} /> : null; })}</div></div> : <div className="min-h-[660px] w-full max-w-[560px] whitespace-pre-wrap bg-white p-7 font-mono text-xs leading-6 text-[var(--ink)] shadow-[0_8px_28px_rgba(17,40,62,0.14)]">{document.ocrText || "กำลังเตรียมตัวอย่างข้อมูล..."}</div>}</div><div className="border-t border-[#d5dee7] bg-white px-4 py-3 text-xs text-[var(--muted)]">กรอบฟ้า = OCR · กรอบเหลือง = รายชื่อ · กรอบเขียว = ฟิลด์สกัด · กรอบชมพู = รายการที่เลือก</div></section>
      <section className="min-w-0 overflow-hidden rounded-xl border border-[var(--line)] bg-white"><div className="flex flex-col gap-3 border-b border-[var(--line)] p-5 sm:flex-row sm:items-start sm:justify-between"><div><p className="text-[11px] font-bold uppercase tracking-[0.15em] text-[var(--accent)]">ข้อมูลจากการสกัด</p><h2 className="mt-1 text-lg font-bold text-[var(--ink)]">ตรวจสอบข้อมูลเอกสาร</h2><p className="mt-1 text-sm text-[var(--muted)]">แก้ไขได้ทุกช่อง และระบบจะแจ้งเมื่อชื่อไม่ตรงกับ directory_users</p></div>{lowConfidenceCount > 0 && <div className="flex shrink-0 items-center gap-2 rounded-lg bg-[var(--warning-soft)] px-3 py-2 text-xs font-semibold text-[var(--warning)]"><WarningCircle size={16} weight="fill" /> มี {lowConfidenceCount} รายการควรตรวจสอบ</div>}</div>
        <div className="flex overflow-x-auto border-b border-[var(--line)] px-2"><button type="button" className={tabClass("members")} onClick={() => setPanel("members")}>รายนาม <span className="ml-1 rounded-full bg-[var(--accent-soft)] px-2 py-0.5 text-[11px]">{appointments.length}</span></button><button type="button" className={tabClass("metadata")} onClick={() => setPanel("metadata")}>ข้อมูลคำสั่ง</button><button type="button" className={tabClass("duties")} onClick={() => setPanel("duties")}>หน้าที่รับผิดชอบ</button></div>
        {panel === "members" && <div><div className="flex flex-col gap-3 border-b border-[var(--line)] bg-[#fbfcfd] p-4"><div className="relative"><MagnifyingGlass size={17} className="absolute left-3 top-3 text-[var(--muted)]" /><input value={memberQuery} onChange={(event) => setMemberQuery(event.target.value)} placeholder="ค้นหาฝ่าย ชื่อ ตำแหน่ง หรือบทบาท" className="focus-ring h-10 w-full rounded-lg border border-[var(--line)] bg-white pl-9 pr-3 text-sm outline-none focus:border-[var(--accent)]" /></div><div className="flex flex-wrap gap-2 text-xs font-semibold"><span className="mr-1 self-center text-[var(--muted)]">กรอง:</span><button type="button" onClick={() => setMemberFilter("all")} className={filterClass("all")}>ทั้งหมด</button><button type="button" onClick={() => setMemberFilter("warning")} className={filterClass("warning")}>ต้องตรวจสอบรายชื่อ</button><button type="button" onClick={() => setMemberFilter("pending")} className={filterClass("pending")}>รอตรวจสอบ</button><button type="button" onClick={() => setMemberFilter("verified")} className={filterClass("verified")}>ตรวจแล้ว</button></div></div><div className="max-h-[680px] overflow-auto p-3">{departmentGroups.map(([department, rows]) => { const duty = rows.find((row) => row.responsibilities?.trim())?.responsibilities || "ยังไม่พบหน้าที่ของฝ่ายนี้จาก PDF"; return <section key={department} className="mb-4 overflow-hidden rounded-lg border border-[var(--line)] last:mb-0"><div className="border-b border-[var(--line)] bg-[#f7fafc] px-4 py-3"><div className="flex flex-wrap items-baseline justify-between gap-2"><h3 className="text-sm font-bold text-[var(--ink)]">{department}</h3><span className="text-xs font-semibold tabular-nums text-[var(--muted)]">{rows.length} รายชื่อ</span></div><p className="mt-1 text-xs leading-5 text-[var(--muted)]"><span className="font-semibold text-[var(--ink)]">หน้าที่ฝ่าย:</span> {duty}</p></div><div className="overflow-x-auto"><table className="data-grid w-full min-w-[760px] text-left text-sm"><thead><tr><th className="w-10 px-3 py-2">✓</th><th className="px-3 py-2">รายชื่อ</th><th className="px-3 py-2">ตำแหน่ง</th><th className="px-3 py-2">บทบาท</th><th className="px-3 py-2">ความแม่นยำและสถานะในระบบ</th><th className="px-3 py-2">เอกสาร</th></tr></thead><tbody>{rows.map((row) => { const state = matchState(row); const warning = state.tone !== "good"; return <tr key={row.id} className={selectedAppointmentId === row.id ? "bg-rose-50" : warning ? "bg-amber-50/50" : "group"}><td className="px-3 py-3 align-top"><input type="checkbox" checked={row.verified} onChange={(event) => updateAppointment(row.id, "verified", event.target.checked)} aria-label={`ยืนยัน ${row.fullName}`} className="mt-1 accent-[var(--success)]" /></td><td className="px-3 py-3 align-top"><input value={row.fullName} onChange={(event) => updateAppointment(row.id, "fullName", event.target.value)} className="focus-ring w-full min-w-[180px] rounded border border-transparent px-1 py-1 font-semibold text-[var(--ink)] outline-none focus:border-[var(--accent)]" /><span className="mt-1 block text-[11px] text-[var(--muted)]">{row.directoryUser?.fullName ? `ชื่อในระบบ: ${row.directoryUser.fullName}` : "แก้ไขหรือยอมรับข้อมูลนี้ได้"}</span></td><td className="px-3 py-3 align-top"><input value={row.position} onChange={(event) => updateAppointment(row.id, "position", event.target.value)} className="focus-ring w-full min-w-[140px] rounded border border-transparent px-1 py-1 text-[var(--muted)] outline-none focus:border-[var(--accent)]" /></td><td className="px-3 py-3 align-top"><input value={row.committeeRole} onChange={(event) => updateAppointment(row.id, "committeeRole", event.target.value)} className="focus-ring w-full min-w-[120px] rounded border border-transparent px-1 py-1 text-[var(--muted)] outline-none focus:border-[var(--accent)]" /></td><td className="px-3 py-3 align-top"><div className="flex flex-col items-start gap-1"><span className={`font-semibold tabular-nums ${row.confidence >= 0.9 ? "text-[var(--success)]" : "text-[var(--warning)]"}`}>{Math.round(row.confidence * 100)}% · จากเอกสาร</span><span className={`inline-flex rounded-md px-2 py-1 text-[11px] font-bold ${state.tone === "good" ? "bg-[var(--success-soft)] text-[var(--success)]" : state.tone === "warning" ? "bg-[var(--warning-soft)] text-[var(--warning)]" : "bg-[var(--danger-soft)] text-[var(--danger)]"}`}>{state.label}</span>{warning && <p className="max-w-[190px] text-[11px] leading-4 text-[var(--warning)]">ควรตรวจสอบหรือแก้ไขก่อนยืนยัน</p>}<span className="text-[11px] text-[var(--muted)]">หน้าเอกสาร {row.pageNo || "-"}</span></div></td><td className="px-3 py-3 align-top"><button type="button" onClick={() => locateAppointment(row.id, row.pageNo)} className="focus-ring inline-flex items-center gap-1 rounded-md border border-[var(--line)] px-2 py-1.5 text-xs font-semibold text-[var(--accent)] hover:border-[var(--accent)]"><MapPin size={14} /> ดูกรอบ</button></td></tr>; })}</tbody></table></div></section>; })}{departmentGroups.length === 0 && <div className="p-8 text-center text-sm text-[var(--muted)]">ไม่พบรายการตามตัวกรอง</div>}</div><div className="flex items-center gap-2 border-t border-[var(--line)] bg-[#fffaf0] px-5 py-3 text-xs text-[var(--warning)]"><Info size={16} weight="fill" /><span>สีเหลือง/แดงหมายถึงชื่อไม่ตรงหรือไม่พบในระบบ สามารถแก้ไขหรือยอมรับข้อมูลเดิมได้</span></div></div>}
        {panel === "metadata" && <div className="grid gap-4 p-5 sm:grid-cols-2">{metadataFields.map((field) => <label className={`block ${field.fieldKey === "committee_name" ? "sm:col-span-2" : ""}`} key={field.id}><span className="text-xs font-semibold text-[var(--muted)]">{field.fieldLabel}</span><input value={field.value} onChange={(event) => updateField(field.id, event.target.value)} className={`focus-ring mt-2 h-10 w-full rounded-lg border px-3 text-sm font-semibold text-[var(--ink)] outline-none focus:border-[var(--accent)] ${field.confidence < 0.9 || !field.value ? "border-[var(--warning)] bg-[var(--warning-soft)]" : "border-[var(--line)]"}`} /><span className="mt-1 block text-[11px] text-[var(--muted)]">หน้า {field.pageNo || "-"} · ความมั่นใจ {Math.round(field.confidence * 100)}% · {field.sourceText || "ไม่พบข้อความอ้างอิง"}</span></label>)}{metadataFields.length === 0 && <p className="text-sm text-[var(--muted)]">ยังไม่มีข้อมูลคำสั่งจาก OCR</p>}</div>}
        {panel === "duties" && <div className="space-y-5 p-5"><label className="block"><span className="text-sm font-bold text-[var(--ink)]">หน้าที่รับผิดชอบของคณะกรรมการ</span><textarea value={document.responsibilities} onChange={(event) => setDocument((current) => ({ ...current, responsibilities: event.target.value }))} rows={8} className="focus-ring mt-2 w-full rounded-lg border border-[var(--line)] px-3 py-2 text-sm leading-6 text-[var(--ink)] outline-none focus:border-[var(--accent)]" /><span className="mt-1 block text-xs text-[var(--muted)]">แก้ไขข้อความจากเอกสารได้ แล้วกดบันทึกฉบับร่าง</span></label><label className="block"><span className="text-sm font-bold text-[var(--ink)]">การอ้างอิงประกาศเพิ่มเติม</span><textarea value={document.additionalReferences} onChange={(event) => setDocument((current) => ({ ...current, additionalReferences: event.target.value }))} rows={4} className="focus-ring mt-2 w-full rounded-lg border border-[var(--line)] px-3 py-2 text-sm leading-6 text-[var(--ink)] outline-none focus:border-[var(--accent)]" /></label></div>}
        <div className="flex flex-col justify-between gap-3 border-t border-[var(--line)] bg-[#fbfcfd] p-4 sm:flex-row sm:items-center"><div className="flex items-center gap-2 text-xs text-[var(--muted)]">{document.status === "CONFIRMED" ? <><CheckCircle size={18} weight="fill" className="text-[var(--success)]" /> ยืนยันข้อมูลแล้วโดย {document.confirmedBy || "เจ้าหน้าที่"}</> : <><Info size={18} className="text-[var(--accent)]" /> ระบบจะไม่ถือเป็นข้อมูลสมบูรณ์จนกว่าจะยืนยัน</>}</div><div className="flex gap-2"><button type="button" disabled={saving} onClick={() => saveDraft(false)} className="focus-ring inline-flex h-10 items-center justify-center rounded-lg border border-[var(--line)] px-4 text-sm font-semibold text-[var(--muted)] hover:border-[var(--navy)] hover:text-[var(--navy)] disabled:opacity-50">บันทึกฉบับร่าง</button><button type="button" disabled={saving || document.status === "CONFIRMED"} onClick={() => saveDraft(true)} className="focus-ring inline-flex h-10 items-center justify-center gap-2 rounded-lg bg-[var(--success)] px-4 text-sm font-semibold text-white hover:bg-[#1a6f4b] disabled:cursor-not-allowed disabled:bg-[#8ab69f]"><FloppyDisk size={17} />{saving ? "กำลังบันทึก..." : "บันทึกและยืนยันข้อมูล"}</button></div></div>
      </section>
    </div>
    {revisions.length > 0 && <div className="overflow-hidden rounded-xl border border-[var(--line)] bg-white"><div className="border-b border-[var(--line)] p-5"><h2 className="text-base font-bold text-[var(--ink)]">ประวัติการแก้ไข</h2><p className="mt-1 text-sm text-[var(--muted)]">ตรวจสอบย้อนหลังได้จากตาราง `document_revisions`</p></div><div className="overflow-x-auto"><table className="data-grid w-full min-w-[620px] text-left text-sm"><thead><tr><th className="px-5 py-3">ฟิลด์</th><th className="px-4 py-3">ค่าเดิม</th><th className="px-4 py-3">ค่าที่แก้ไข</th><th className="px-5 py-3">ผู้แก้ไข / เวลา</th></tr></thead><tbody>{revisions.map((revision) => <tr key={revision.id}><td className="px-5 py-3 font-semibold text-[var(--ink)]">{revision.fieldKey}</td><td className="max-w-[180px] truncate px-4 py-3 text-[var(--muted)]">{revision.oldValue || "-"}</td><td className="max-w-[180px] truncate px-4 py-3 text-[var(--ink)]">{revision.newValue || "-"}</td><td className="px-5 py-3 text-xs text-[var(--muted)]">{revision.changedBy}<br />{formatThaiDate(revision.changedAt)}</td></tr>)}</tbody></table></div></div>}
  </div></AppShell>;
}
