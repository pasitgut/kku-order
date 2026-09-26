"use client";

import { useRouter } from "next/navigation";
import { ChangeEvent, useEffect, useMemo, useRef, useState } from "react";
import { ArrowCounterClockwise, CaretLeft, CaretRight, Check, CheckCircle, Info, MagnifyingGlass, MapPin, PencilSimple, Trash, UploadSimple, UserPlus, UsersThree, WarningCircle } from "@phosphor-icons/react";
import { AppShell, StatusBadge } from "@/components/app-shell";
import { useToast } from "@/components/toast";
import { ApiDocument, ApiError, Appointment, DirectoryCandidate, DirectoryUser, DocumentRevision, confirmDocument, createDirectoryUser, deleteDocument, formatFileSize, formatThaiDate, getDirectoryUsers, getDocumentPageImageUrl, getDocumentRevisions, replaceDocumentFile, statusLabel, updateDocument } from "@/lib/api";
import { MatchState, MemberFilter, countByFilter, departmentOf, filterMembers, groupByDepartment, isLinked, matchState, rosterRows, summarizeDepartments } from "@/lib/member-review";

type PanelView = "roster" | "members" | "metadata" | "duties";

const metadataKeys = ["order_type", "order_no", "order_year_be", "committee_name", "issued_date", "effective_date", "expiry_date", "signer_name", "signer_position"];

const panelTabs: { value: PanelView; label: string }[] = [
  { value: "roster", label: "รายชื่อทั้งหมด" },
  { value: "members", label: "รายนาม" },
  { value: "metadata", label: "ข้อมูลคำสั่ง" },
  { value: "duties", label: "หน้าที่รับผิดชอบ" },
];

const memberFilters: { value: MemberFilter; label: string }[] = [
  { value: "all", label: "ทั้งหมด" },
  { value: "warning", label: "ต้องตรวจรายชื่อ" },
  { value: "pending", label: "รอตรวจ" },
  { value: "verified", label: "ตรวจแล้ว" },
];

type RestorePoint = { fullName: string; position: string; nameMatchMethod: string; nameMatchScore: number };
type DirectoryPicker = { appointmentId: number; query: string; results: DirectoryUser[]; loading: boolean };
type PersonDraft = { appointmentId: number; firstName: string; lastName: string; positionTitle: string; email: string; saving: boolean; conflictUserId?: number };
type CandidateCompare = { appointment: Appointment; users: DirectoryUser[] | null; selected: number };

const inputClass = "focus-ring h-9 w-full rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-[13px] outline-none focus:border-[var(--accent)]";
const smallButton = "focus-ring inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-md border px-3 text-xs transition";

export function DocumentWorkspace({ document: initialDocument }: { document: ApiDocument }) {
  const router = useRouter();
  const toast = useToast();
  const [document, setDocument] = useState(initialDocument);
  const [fields, setFields] = useState(initialDocument.fields ?? []);
  const [appointments, setAppointments] = useState(initialDocument.appointments ?? []);
  const [revisions, setRevisions] = useState<DocumentRevision[]>([]);
  const [selectedPage, setSelectedPage] = useState(1);
  const [selectedAppointmentId, setSelectedAppointmentId] = useState<number | null>(null);
  const [panel, setPanel] = useState<PanelView>("members");
  const [memberFilter, setMemberFilter] = useState<MemberFilter>("all");
  const [memberQuery, setMemberQuery] = useState("");
  const [departmentFilter, setDepartmentFilter] = useState("");
  const [openRows, setOpenRows] = useState<Record<number, boolean>>({});
  const [saving, setSaving] = useState(false);
  const [deleteArmed, setDeleteArmed] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [picker, setPicker] = useState<DirectoryPicker | null>(null);
  const [draft, setDraft] = useState<PersonDraft | null>(null);
  const [compare, setCompare] = useState<CandidateCompare | null>(null);
  const [restorePoints, setRestorePoints] = useState<Record<number, RestorePoint>>({});
  const [replaceArmed, setReplaceArmed] = useState(false);
  const [replacing, setReplacing] = useState(false);
  const replaceInputRef = useRef<HTMLInputElement>(null);

  const selectedOCRPage = document.ocrPages?.find((page) => page.pageNo === selectedPage);
  const pageWidth = selectedOCRPage?.imageWidth || 1;
  const pageHeight = selectedOCRPage?.imageHeight || 1;
  const metadataFields = fields.filter((field) => metadataKeys.includes(field.fieldKey));
  const lowConfidenceCount = useMemo(() => fields.filter((field) => field.confidence < 0.9 || !field.value).length + appointments.filter((item) => item.confidence < 0.9 || item.nameMatchMethod !== "EXACT").length, [fields, appointments]);

  const departmentSummary = useMemo(() => summarizeDepartments(appointments), [appointments]);
  const activeDepartment = departmentSummary.some((entry) => entry.name === departmentFilter) ? departmentFilter : "";
  const filterCounts = useMemo(() => countByFilter(appointments), [appointments]);
  const unlinkedAppointments = useMemo(() => appointments.filter((row) => !isLinked(row)), [appointments]);
  const verifiedCount = appointments.filter((row) => row.verified).length;
  const departmentGroups = useMemo(() => groupByDepartment(filterMembers(appointments, { filter: memberFilter, department: activeDepartment, query: memberQuery })), [appointments, memberFilter, activeDepartment, memberQuery]);
  const roster = useMemo(() => rosterRows(appointments), [appointments]);

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

  const showError = (reason: unknown, fallback: string) => toast(reason instanceof Error ? reason.message : fallback, { kind: "error" });

  const updateField = (id: number, value: string) => setFields((current) => current.map((field) => field.id === id ? { ...field, value } : field));
  const updateAppointment = (id: number, key: "fullName" | "position" | "committeeRole" | "verified", value: string | boolean) => setAppointments((current) => current.map((item) => item.id === id ? { ...item, [key]: value } : item));
  const removeAppointment = (id: number) => setAppointments((current) => current.filter((item) => item.id !== id));

  const linkAppointment = (id: number, userId: number, fullName: string, positionTitle?: string) => {
    const before = appointments.find((item) => item.id === id);
    if (before && !isLinked(before)) {
      setRestorePoints((points) => ({ ...points, [id]: { fullName: before.fullName, position: before.position, nameMatchMethod: before.nameMatchMethod ?? "NOT_FOUND", nameMatchScore: before.nameMatchScore ?? 0 } }));
    }
    setAppointments((current) => current.map((item) => item.id === id ? { ...item, directoryUserId: userId, fullName: fullName || item.fullName, position: item.position || positionTitle || "", nameMatchMethod: "MANUAL", nameMatchScore: 1 } : item));
    setPicker(null);
    setDraft(null);
    setCompare(null);
    toast(`ผูกกับ ${fullName || before?.fullName || "บุคคลในระบบ"} แล้ว · กดบันทึกเพื่อเก็บลงระบบ`);
  };

  // เลือกผิดคนได้ จึงต้องถอยกลับไปยังชื่อที่ OCR อ่านมาได้เสมอ
  const unlinkAppointment = (id: number) => {
    const before = restorePoints[id];
    setAppointments((current) => current.map((item) => item.id === id ? {
      ...item,
      directoryUserId: undefined,
      directoryUser: undefined,
      fullName: before?.fullName ?? item.fullName,
      position: before?.position ?? item.position,
      nameMatchMethod: before?.nameMatchMethod ?? "NOT_FOUND",
      nameMatchScore: before?.nameMatchScore ?? 0,
    } : item));
    setRestorePoints((points) => {
      const next = { ...points };
      delete next[id];
      return next;
    });
    setOpenRows((rows) => ({ ...rows, [id]: true }));
  };

  const toggleGroupVerified = (department: string, rows: Appointment[], checked: boolean) => {
    const ids = new Set(rows.map((row) => row.id));
    setAppointments((current) => current.map((item) => ids.has(item.id) ? { ...item, verified: checked } : item));
    if (checked) toast(`ตรวจแล้วทั้งฝ่าย · ${department}`);
  };

  const runDirectorySearch = async (appointmentId: number, query: string) => {
    setPicker({ appointmentId, query, results: [], loading: true });
    try {
      const response = await getDirectoryUsers(query.trim());
      setPicker({ appointmentId, query, results: response.data, loading: false });
    } catch (reason) {
      showError(reason, "ค้นหารายชื่อไม่สำเร็จ");
      setPicker({ appointmentId, query, results: [], loading: false });
    }
  };

  const openDraft = (row: Appointment) => {
    const parts = row.fullName.trim().split(/\s+/).filter(Boolean);
    setDraft({
      appointmentId: row.id,
      firstName: parts.length > 1 ? parts.slice(0, -1).join(" ") : row.fullName.trim(),
      lastName: parts.length > 1 ? parts[parts.length - 1] : "",
      positionTitle: row.position ?? "",
      email: "",
      saving: false,
    });
    setPicker(null);
  };

  const submitDraft = async () => {
    if (!draft) return;
    setDraft({ ...draft, saving: true, conflictUserId: undefined });
    try {
      const response = await createDirectoryUser({ firstName: draft.firstName.trim(), lastName: draft.lastName.trim(), positionTitle: draft.positionTitle.trim(), email: draft.email.trim() });
      linkAppointment(draft.appointmentId, response.data.id, `${response.data.firstName} ${response.data.lastName}`.trim(), response.data.positionTitle);
    } catch (reason) {
      // backend ไม่สร้างชื่อซ้ำ และส่ง id ของคนเดิมกลับมาให้ผูกแทนได้ทันที
      if (reason instanceof ApiError && reason.status === 409 && typeof reason.details.directoryUserId === "number") {
        const conflictUserId = reason.details.directoryUserId;
        setDraft((current) => current ? { ...current, saving: false, conflictUserId } : current);
        return;
      }
      showError(reason, "ไม่สามารถสร้างบุคคลใหม่ได้");
      setDraft((current) => current ? { ...current, saving: false } : current);
    }
  };

  const openCompare = async (row: Appointment) => {
    const candidates = row.candidates ?? [];
    setCompare({ appointment: row, users: null, selected: candidates[0]?.userId ?? 0 });
    try {
      const response = await getDirectoryUsers(candidates[0]?.fullName ?? row.fullName);
      const ids = new Set(candidates.map((candidate) => candidate.userId));
      setCompare((current) => current && current.appointment.id === row.id ? { ...current, users: response.data.filter((user) => ids.has(user.id)) } : current);
    } catch {
      setCompare((current) => current && current.appointment.id === row.id ? { ...current, users: [] } : current);
    }
  };

  const locateAppointment = (id: number, pageNo: number) => { setSelectedAppointmentId(id); setSelectedPage(Math.max(1, pageNo || 1)); };
  const toggleRow = (id: number, open: boolean) => setOpenRows((rows) => ({ ...rows, [id]: !open }));
  const showWarnings = () => { setPanel("members"); setMemberFilter("warning"); setDepartmentFilter(""); setMemberQuery(""); setOpenRows({}); };

  const buildPayload = () => ({
    responsibilities: document.responsibilities,
    additionalReferences: document.additionalReferences,
    fields: fields.map((field) => ({ id: field.id, fieldKey: field.fieldKey, fieldLabel: field.fieldLabel, value: field.value, verified: field.verified })),
    appointments: appointments.map((item) => ({ id: item.id, directoryUserId: item.directoryUserId, fullName: item.fullName, position: item.position, department: item.department, committeeRole: item.committeeRole, responsibilities: item.responsibilities, confidence: item.confidence, pageNo: item.pageNo, boundingBox: item.boundingBox, verified: item.verified })),
  });

  const saveDraft = async (confirm = false) => {
    setSaving(true);
    try {
      const response = await updateDocument(document.id, { ...buildPayload(), reviewNotes: document.reviewNotes });
      setDocument(response.data); setFields(response.data.fields ?? fields); setAppointments(response.data.appointments ?? appointments); setRestorePoints({});
      if (confirm) {
        await confirmDocument(document.id);
        setDocument((current) => ({ ...current, status: "CONFIRMED", confirmedAt: new Date().toISOString(), confirmedBy: "local-user" }));
        toast("บันทึกและยืนยันข้อมูลเรียบร้อยแล้ว");
      } else toast("บันทึกฉบับร่างลงฐานข้อมูลแล้ว");
    } catch (reason) {
      showError(reason, "ไม่สามารถบันทึกข้อมูลได้");
    } finally {
      setSaving(false);
    }
  };

  // ทับไฟล์แล้วข้อมูลที่สกัดไว้ทั้งหมดถูกสร้างใหม่ จึงโหลดหน้าใหม่เพื่อเริ่มติดตามคิวประมวลผลอีกครั้ง
  const onReplaceFileChosen = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    setReplacing(true);
    try {
      await replaceDocumentFile(document.id, file);
      window.location.reload();
    } catch (reason) {
      showError(reason, "อัปโหลดไฟล์ทับไม่สำเร็จ");
      setReplacing(false);
      setReplaceArmed(false);
    }
  };

  const removeDocument = async () => {
    if (!deleteArmed) {
      setDeleteArmed(true);
      return;
    }
    setDeleting(true);
    try {
      await deleteDocument(document.id);
      router.replace("/documents");
      router.refresh();
    } catch (reason) {
      showError(reason, "ไม่สามารถลบข้อมูลได้");
      setDeleteArmed(false);
    } finally {
      setDeleting(false);
    }
  };

  const bandMeta = <>
    <StatusBadge status={statusLabel(document.status)} />
    <span className="text-[13px] text-[var(--band-text)]">นำเข้า {formatThaiDate(document.createdAt)}</span>
    <button type="button" onClick={removeDocument} disabled={deleting} className={`focus-ring inline-flex h-10 items-center gap-1.5 rounded-md px-3.5 text-[13px] font-medium transition ${deleteArmed ? "bg-[var(--danger)] text-white" : "border border-white/45 text-white hover:bg-white/10"}`}><Trash size={15} />{deleting ? "กำลังลบ..." : deleteArmed ? "ยืนยันการลบ" : "ลบข้อมูลทดสอบ"}</button>
    {deleteArmed && !deleting && <button type="button" onClick={() => setDeleteArmed(false)} className="focus-ring h-10 rounded-md px-2 text-[13px] text-[var(--band-text)] hover:text-white">ยกเลิก</button>}
  </>;

  return <AppShell backHref="/documents" breadcrumb="เอกสารทั้งหมด / ตรวจสอบข้อมูล" title={document.title || document.originalFilename} meta={bandMeta}><div className="space-y-6">
    <div className="grid items-stretch gap-6 xl:grid-cols-2">
      <section className="flex min-h-[420px] flex-col overflow-hidden rounded-[10px] bg-[#e9eef3] xl:min-h-[720px] shadow-[var(--shadow-card)]">
        <div className="flex items-center justify-between border-b border-[var(--line)] bg-white py-3 pl-5 pr-3"><div><p className="text-sm font-semibold text-[var(--navy)]">ต้นฉบับเอกสาร</p><p className="mt-0.5 text-xs font-light text-[var(--muted)]">{document.pageCount || "-"} หน้า · {formatFileSize(document.fileSizeBytes)} · {document.ocrProvider || "นำเข้าข้อมูลโดยตรง"}</p></div><div className="flex items-center gap-1"><button type="button" onClick={() => setSelectedPage((page) => Math.max(1, page - 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)]" aria-label="หน้าก่อนหน้า"><CaretLeft size={17} /></button><span className="px-1.5 text-[13px]">หน้า {selectedPage} / {document.pageCount || 1}</span><button type="button" onClick={() => setSelectedPage((page) => Math.min(document.pageCount || 1, page + 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)]" aria-label="หน้าถัดไป"><CaretRight size={17} /></button></div></div>
        <div className="shell-scrollbar flex flex-1 items-start justify-center overflow-auto p-6">{document.sourceType.toLowerCase() === "pdf" ? <div className="relative w-full max-w-[600px] overflow-hidden bg-white shadow-[0_8px_28px_rgba(17,40,62,0.14)]"><img src={getDocumentPageImageUrl(document.id, selectedPage)} alt={`หน้า ${selectedPage} ของ ${document.title}`} className="block h-auto w-full" /><div className="pointer-events-none absolute inset-0">{(selectedOCRPage?.lines ?? []).map((line) => { const box = parseBox(line.boundingBox); return box ? <span key={line.id} title={line.text} className="absolute border border-sky-400/50 bg-sky-300/5" style={box} /> : null; })}{appointments.filter((item) => item.pageNo === selectedPage).map((item) => { const box = parseBox(item.boundingBox); return box ? <span key={`appointment-${item.id}`} title={item.fullName} className={`absolute ${selectedAppointmentId === item.id ? "z-10 border-2 border-dashed border-[var(--highlight)] bg-[rgba(240,72,20,0.1)]" : "border-2 border-amber-500 bg-amber-300/20"}`} style={box} /> : null; })}{fields.filter((field) => field.pageNo === selectedPage).map((field) => { const box = parseBox(field.boundingBox); return box ? <span key={`field-${field.id}`} title={field.fieldLabel} className="absolute border-2 border-emerald-500 bg-emerald-300/20" style={box} /> : null; })}</div></div> : <div className="min-h-[660px] w-full max-w-[600px] whitespace-pre-wrap bg-white p-7 font-mono text-xs leading-6 shadow-[0_8px_28px_rgba(17,40,62,0.14)]">{document.ocrText || "กำลังเตรียมตัวอย่างข้อมูล..."}</div>}</div>
        <div className="border-t border-[var(--line)] bg-white px-5 py-2.5 text-[11px] text-[var(--muted)]">กรอบฟ้า = OCR · กรอบเหลือง = รายชื่อ · กรอบเขียว = ฟิลด์สกัด · กรอบส้มเส้นประ = รายการที่เลือก</div>
      </section>

      <section className="card flex min-w-0 flex-col overflow-hidden">
        <div className="space-y-3 px-5 pb-4 pt-5 lg:px-6">
          <div className="flex flex-wrap items-baseline justify-between gap-2"><h2 className="text-lg font-semibold text-[var(--navy)]">ตรวจสอบข้อมูลเอกสาร</h2><span className="text-xs text-[var(--muted)]">ข้อมูลจากการสกัด · แก้ไขได้ทุกช่อง</span></div>
          <div role="status" className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <div className="min-w-[180px] flex-1"><p className="text-xs text-[var(--muted)]">ตรวจแล้ว <span className="font-medium text-[var(--ink)]">{verifiedCount}</span> จาก {appointments.length} รายชื่อ</p><span className="mt-1.5 block h-1 overflow-hidden rounded-full bg-[var(--line)]"><span className="block h-full rounded-full bg-[var(--accent)] transition-all" style={{ width: `${appointments.length ? Math.round((verifiedCount / appointments.length) * 100) : 0}%` }} /></span></div>
            {unlinkedAppointments.length > 0 && <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs"><span className="h-2 w-2 rounded-full bg-[var(--highlight)]" />ยังไม่ผูก {unlinkedAppointments.length}</span>}
            {lowConfidenceCount > 0 && <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs"><span className="h-2 w-2 rounded-full bg-[var(--warning-dot)]" />ควรตรวจ {lowConfidenceCount}</span>}
            {filterCounts.warning > 0 && <button type="button" onClick={showWarnings} className="focus-ring h-8 whitespace-nowrap rounded text-xs font-medium text-[var(--accent)] hover:underline">ดูรายการที่ต้องแก้ →</button>}
          </div>
        </div>

        <div role="tablist" aria-label="ส่วนของข้อมูลเอกสาร" className="flex overflow-x-auto border-y border-[var(--line)] px-3">{panelTabs.map((tab) => <button key={tab.value} type="button" role="tab" aria-selected={panel === tab.value} onClick={() => setPanel(tab.value)} className={`focus-ring flex h-12 items-center gap-2 whitespace-nowrap px-3.5 text-sm transition ${panel === tab.value ? "font-medium text-[var(--accent)] shadow-[inset_0_-3px_0_var(--accent)]" : "text-[var(--muted)] hover:text-[var(--ink)]"}`}>{tab.label}{tab.value === "roster" && <span className={`rounded-full px-2 py-0.5 text-[11px] ${panel === tab.value ? "bg-[var(--accent-soft)]" : "bg-[var(--line-soft)]"}`}>{appointments.length}</span>}</button>)}</div>

        {panel === "roster" && <div className="flex-1">
          <div className="flex items-baseline justify-between px-5 py-3.5 lg:px-6"><p className="text-sm font-semibold text-[var(--navy)]">รายชื่อทั้งหมดในคำสั่ง</p><p className="text-xs text-[var(--muted)]">{appointments.length} รายชื่อ · {departmentSummary.length} ฝ่าย</p></div>
          <div className="overflow-x-auto"><table className="data-grid w-full min-w-[520px] text-left text-sm"><thead><tr><th className="w-12 px-5 py-2.5 lg:pl-6">#</th><th className="px-3 py-2.5">ชื่อ</th><th className="px-3 py-2.5">ฝ่าย</th><th className="px-5 py-2.5 lg:pr-6">ตำแหน่ง</th></tr></thead><tbody>{roster.map((row) => <tr key={row.id}><td className="px-5 py-3 text-xs text-[var(--subtle)] lg:pl-6">{row.no}</td><td className="px-3 py-3 font-medium">{row.fullName || "-"}</td><td className="px-3 py-3">{row.department}</td><td className="px-5 py-3 text-[var(--muted)] lg:pr-6">{row.position || "-"}</td></tr>)}{roster.length === 0 && <tr><td colSpan={4} className="px-5 py-12 text-center text-sm text-[var(--muted)]">ยังไม่มีรายชื่อจากเอกสาร</td></tr>}</tbody></table></div>
        </div>}

        {panel === "members" && <div className="flex-1">
          <div className="space-y-2.5 border-b border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 lg:px-6">
            <div className="relative"><MagnifyingGlass size={16} className="absolute left-3.5 top-1/2 -translate-y-1/2 text-[var(--muted)]" /><input value={memberQuery} onChange={(event) => setMemberQuery(event.target.value)} aria-label="ค้นหารายชื่อ" placeholder="ค้นหาฝ่าย ชื่อ ตำแหน่ง หรือบทบาท" className="focus-ring h-10 w-full rounded-full border border-[var(--line-strong)] bg-white pl-10 pr-4 text-[13px] outline-none focus:border-[var(--accent)]" /></div>
            <div role="group" aria-label="กรองตามสถานะ" className="flex flex-wrap items-center gap-1.5"><span className="w-11 text-xs text-[var(--muted)]">สถานะ</span>{memberFilters.map((item) => <button key={item.value} type="button" aria-pressed={memberFilter === item.value} onClick={() => { setMemberFilter(item.value); setOpenRows({}); }} className={`focus-ring h-8 rounded-full border px-3 text-xs transition ${memberFilter === item.value ? "border-[var(--accent)] bg-[var(--accent)] font-medium text-white" : "border-[var(--line-strong)] bg-white hover:border-[var(--accent)] hover:text-[var(--accent)]"}`}>{item.label} <span className={memberFilter === item.value ? "" : "text-[var(--muted)]"}>{filterCounts[item.value]}</span></button>)}</div>
            {departmentSummary.length > 1 && <div role="group" aria-label="กรองตามฝ่าย" className="flex flex-wrap items-center gap-1.5"><span className="w-11 text-xs text-[var(--muted)]">ฝ่าย</span>{[{ name: "", label: "ทุกฝ่าย", total: appointments.length, unlinked: 0 }, ...departmentSummary.map((entry) => ({ ...entry, label: entry.name }))].map((entry) => <button key={entry.name || "all"} type="button" title={entry.label} aria-pressed={activeDepartment === entry.name} onClick={() => setDepartmentFilter(entry.name)} className={`focus-ring inline-flex h-8 max-w-full items-center gap-1.5 rounded-full border px-3 text-xs transition ${activeDepartment === entry.name ? "border-[var(--accent)] bg-[var(--accent-soft)] font-medium text-[#155e97]" : "border-[var(--line-strong)] bg-white hover:border-[var(--accent)] hover:text-[var(--accent)]"}`}><span className="max-w-[190px] truncate">{entry.label}</span><span className="tabular-nums text-[var(--muted)]">{entry.total}</span>{entry.unlinked > 0 && <span aria-label={`ยังไม่ผูก ${entry.unlinked}`} className="h-1.5 w-1.5 rounded-full bg-[var(--highlight)]" />}</button>)}</div>}
          </div>

          <div>{departmentGroups.map(([department, rows]) => {
            const duty = rows.find((row) => row.responsibilities?.trim())?.responsibilities || "ยังไม่พบหน้าที่ของฝ่ายนี้จาก PDF";
            const unlinked = appointments.filter((row) => departmentOf(row) === department && !isLinked(row)).length;
            return <section key={department}>
              <div className="flex items-start gap-3 border-b border-[var(--line)] bg-[var(--canvas)] px-5 py-3 lg:px-6">
                <input type="checkbox" aria-label={`ตรวจแล้วทั้งฝ่าย ${department}`} checked={rows.length > 0 && rows.every((row) => row.verified)} ref={(element) => { if (element) element.indeterminate = rows.some((row) => row.verified) && !rows.every((row) => row.verified); }} onChange={(event) => toggleGroupVerified(department, rows, event.target.checked)} className="mt-0.5 h-[18px] w-[18px] shrink-0 accent-[var(--accent)]" />
                <div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-x-2"><h3 className="text-sm font-semibold text-[var(--navy)]">{department}</h3><span className="text-xs text-[var(--muted)]">{rows.length} รายชื่อ</span>{unlinked > 0 && <span className="inline-flex items-center gap-1.5 text-xs text-[var(--muted)]"><span className="h-1.5 w-1.5 rounded-full bg-[var(--highlight)]" />ยังไม่ผูก {unlinked}</span>}</div><p className="mt-0.5 line-clamp-2 text-xs font-light leading-5 text-[var(--muted)]">หน้าที่ฝ่าย: {duty}</p></div>
              </div>
              <ul>{rows.map((row) => {
                const state = matchState(row);
                const problem = state.tone !== "good";
                const open = openRows[row.id] ?? (memberFilter === "warning" && problem);
                return <li key={row.id} className={`border-b border-[var(--line-soft)] ${selectedAppointmentId === row.id ? "bg-[var(--brand-blue-pale)]" : ""}`}>
                  <div className="flex min-h-[52px] items-center gap-3 px-5 py-2 lg:px-6">
                    <input type="checkbox" checked={row.verified} onChange={(event) => updateAppointment(row.id, "verified", event.target.checked)} aria-label={`ตรวจแล้ว ${row.fullName}`} className="h-[18px] w-[18px] shrink-0 accent-[var(--accent)]" />
                    <div className="min-w-0 flex-1 sm:flex sm:items-baseline sm:gap-2.5"><p className="truncate text-sm font-medium sm:max-w-[60%] sm:shrink-0">{row.fullName || "ไม่ระบุชื่อ"}</p><p className="truncate text-xs font-light text-[var(--muted)]">{[row.position, row.committeeRole].filter(Boolean).join(" · ") || "-"}</p></div>
                    <span className="hidden whitespace-nowrap text-xs text-[var(--subtle)] sm:inline">{Math.round(row.confidence * 100)}% · น.{row.pageNo || "-"}</span>
                    <MatchTag state={state} />
                    {problem
                      ? <button type="button" aria-expanded={open} onClick={() => toggleRow(row.id, open)} className={`${smallButton} h-8 border-[var(--line-strong)] bg-white font-medium text-[var(--accent)] hover:bg-[var(--line-soft)]`}>{open ? "ซ่อน" : "จัดการ"}</button>
                      : <button type="button" aria-expanded={open} aria-label={`${open ? "ปิด" : "แก้ไข"} ${row.fullName}`} onClick={() => toggleRow(row.id, open)} className="focus-ring flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--accent)]"><PencilSimple size={15} /></button>}
                  </div>
                  {open && <div className="space-y-3 px-5 pb-4 lg:pr-6 sm:pl-[52px] lg:pl-[54px]">
                    <div className="grid gap-2 sm:grid-cols-3">
                      <label className="block"><span className="text-[11px] text-[var(--muted)]">ชื่อตามเอกสาร</span><input value={row.fullName} onChange={(event) => updateAppointment(row.id, "fullName", event.target.value)} className={`${inputClass} mt-1`} /></label>
                      <label className="block"><span className="text-[11px] text-[var(--muted)]">ตำแหน่ง</span><input value={row.position} onChange={(event) => updateAppointment(row.id, "position", event.target.value)} className={`${inputClass} mt-1`} /></label>
                      <label className="block"><span className="text-[11px] text-[var(--muted)]">บทบาท</span><input value={row.committeeRole} onChange={(event) => updateAppointment(row.id, "committeeRole", event.target.value)} className={`${inputClass} mt-1`} /></label>
                    </div>
                    {isLinked(row)
                      ? <p className="flex flex-wrap items-center gap-2 text-xs text-[var(--muted)]"><Check size={14} className="text-[var(--success)]" />{row.directoryUser?.fullName ? `ผูกกับ ${row.directoryUser.fullName}` : "ผูกกับบุคคลในระบบแล้ว"} · {state.detail}<button type="button" onClick={() => unlinkAppointment(row.id)} className={`${smallButton} h-8 border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--line-soft)]`}><ArrowCounterClockwise size={13} /> เปลี่ยนคน</button></p>
                      : <UnlinkedResolver row={row} state={state} picker={picker} draft={draft} onPick={linkAppointment} onCompare={() => openCompare(row)} onPickerChange={setPicker} onSearch={runDirectorySearch} onDraftChange={setDraft} onOpenDraft={() => openDraft(row)} onSubmitDraft={submitDraft} />}
                    <div className="flex gap-2"><button type="button" onClick={() => locateAppointment(row.id, row.pageNo)} className={`${smallButton} border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--line-soft)]`}><MapPin size={14} /> ดูกรอบในเอกสาร</button><button type="button" onClick={() => removeAppointment(row.id)} className={`${smallButton} border-transparent text-[var(--muted)] hover:text-[var(--danger)]`}><Trash size={13} /> ลบแถว</button></div>
                  </div>}
                </li>;
              })}</ul>
            </section>;
          })}{departmentGroups.length === 0 && <div className="p-10 text-center text-sm text-[var(--muted)]">ไม่พบรายการตามตัวกรอง</div>}</div>
        </div>}

        {panel === "metadata" && <div className="grid flex-1 content-start gap-4 p-5 sm:grid-cols-2 lg:p-6">{metadataFields.map((field) => <label className={`block ${field.fieldKey === "committee_name" ? "sm:col-span-2" : ""}`} key={field.id}><span className="text-xs text-[var(--muted)]">{field.fieldLabel}</span><input value={field.value} onChange={(event) => updateField(field.id, event.target.value)} className={`focus-ring mt-1.5 h-10 w-full rounded-md border px-3 text-sm font-medium outline-none focus:border-[var(--accent)] ${field.confidence < 0.9 || !field.value ? "border-[var(--warning-dot)] bg-[#fffbf0]" : "border-[var(--line-strong)]"}`} /><span className="mt-1 block text-[11px] text-[var(--muted)]">หน้า {field.pageNo || "-"} · ความมั่นใจ {Math.round(field.confidence * 100)}% · {field.sourceText || "ไม่พบข้อความอ้างอิง"}</span></label>)}{metadataFields.length === 0 && <p className="text-sm text-[var(--muted)]">ยังไม่มีข้อมูลคำสั่งจาก OCR</p>}</div>}

        {panel === "duties" && <div className="flex-1 space-y-5 p-5 lg:p-6"><label className="block"><span className="text-sm font-medium text-[var(--navy)]">หน้าที่รับผิดชอบของคณะกรรมการ</span><textarea value={document.responsibilities} onChange={(event) => setDocument((current) => ({ ...current, responsibilities: event.target.value }))} rows={8} className="focus-ring mt-2 w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6 outline-none focus:border-[var(--accent)]" /><span className="mt-1 block text-xs text-[var(--muted)]">แก้ไขข้อความจากเอกสารได้ แล้วกดบันทึกฉบับร่าง</span></label><label className="block"><span className="text-sm font-medium text-[var(--navy)]">การอ้างอิงประกาศเพิ่มเติม</span><textarea value={document.additionalReferences} onChange={(event) => setDocument((current) => ({ ...current, additionalReferences: event.target.value }))} rows={4} className="focus-ring mt-2 w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6 outline-none focus:border-[var(--accent)]" /></label></div>}

        <div className="flex flex-col gap-3 border-t border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 lg:px-6">
          <div className="flex items-start gap-2 text-[13px] leading-5">{replaceArmed ? <><WarningCircle size={18} weight="fill" className="shrink-0 text-[var(--danger)]" /><span className="text-[var(--danger)]">ไฟล์ใหม่จะแทนที่ไฟล์เดิม ข้อมูลที่ตรวจไว้จะถูกสกัดใหม่ทั้งหมด</span><button type="button" onClick={() => setReplaceArmed(false)} className="focus-ring shrink-0 text-[var(--muted)] underline hover:text-[var(--ink)]">ยกเลิก</button></> : unlinkedAppointments.length > 0 ? <><WarningCircle size={18} weight="fill" className="shrink-0 text-[var(--danger)]" /><span className="font-medium text-[var(--danger)]">เหลือ {unlinkedAppointments.length} รายชื่อที่ยังไม่ได้ผูกกับบุคคล</span></> : document.status === "CONFIRMED" ? <><CheckCircle size={18} weight="fill" className="shrink-0 text-[var(--success)]" /><span>ยืนยันข้อมูลแล้วโดย {document.confirmedBy || "เจ้าหน้าที่"}</span></> : <><Info size={18} className="shrink-0 text-[var(--accent)]" /><span className="text-[var(--muted)]">ระบบจะไม่ถือเป็นข้อมูลสมบูรณ์จนกว่าจะยืนยัน</span></>}</div>
          <div className="flex flex-wrap justify-end gap-2"><input ref={replaceInputRef} id="replace-document-file" type="file" accept=".pdf,.docx,.xlsx,.csv" className="sr-only" onChange={onReplaceFileChosen} /><button type="button" disabled={replacing || saving} onClick={() => { if (!replaceArmed) { setReplaceArmed(true); return; } replaceInputRef.current?.click(); }} className={`focus-ring inline-flex h-11 items-center gap-2 whitespace-nowrap rounded-md px-4 text-sm transition disabled:opacity-50 ${replaceArmed ? "bg-[var(--danger)] font-medium text-white" : "text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--ink)]"}`}><UploadSimple size={17} />{replacing ? "กำลังอัปโหลด..." : replaceArmed ? "เลือกไฟล์ที่จะทับ" : "อัปโหลดไฟล์ใหม่ทับ"}</button><button type="button" disabled={saving} onClick={() => saveDraft(false)} className="focus-ring inline-flex h-11 items-center whitespace-nowrap rounded-md border border-[var(--accent)] bg-white px-4 text-sm font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)] disabled:opacity-50">บันทึกฉบับร่าง</button><button type="button" disabled={saving || document.status === "CONFIRMED" || unlinkedAppointments.length > 0} title={unlinkedAppointments.length > 0 ? `ยังมี ${unlinkedAppointments.length} รายชื่อที่ยังไม่ได้ผูกกับบุคคลในระบบ` : undefined} onClick={() => saveDraft(true)} className="focus-ring inline-flex h-11 items-center whitespace-nowrap rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{saving ? "กำลังบันทึก..." : "บันทึกและยืนยันข้อมูล"}</button></div>
        </div>
      </section>
    </div>

    {revisions.length > 0 && <section className="card overflow-hidden"><div className="border-b border-[var(--line)] px-5 py-4 lg:px-6"><h2 className="text-base font-semibold text-[var(--navy)]">ประวัติการแก้ไข</h2><p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">ตรวจสอบย้อนหลังได้จากตาราง `document_revisions`</p></div><div className="overflow-x-auto"><table className="data-grid w-full min-w-[620px] text-left text-sm"><thead><tr><th className="px-6 py-3">ฟิลด์</th><th className="px-4 py-3">ค่าเดิม</th><th className="px-4 py-3">ค่าที่แก้ไข</th><th className="px-6 py-3">ผู้แก้ไข / เวลา</th></tr></thead><tbody>{revisions.map((revision) => <tr key={revision.id}><td className="px-6 py-3 font-medium">{revision.fieldKey}</td><td className="max-w-[180px] truncate px-4 py-3 text-[var(--muted)]">{revision.oldValue || "-"}</td><td className="max-w-[180px] truncate px-4 py-3">{revision.newValue || "-"}</td><td className="px-6 py-3 text-xs text-[var(--muted)]">{revision.changedBy}<br />{formatThaiDate(revision.changedAt)}</td></tr>)}</tbody></table></div></section>}

    {compare && <CandidateCompareDialog compare={compare} onSelect={(userId) => setCompare({ ...compare, selected: userId })} onClose={() => setCompare(null)} onConfirm={(candidate) => linkAppointment(compare.appointment.id, candidate.userId, candidate.fullName, candidate.positionTitle)} onCreate={() => { const row = compare.appointment; setCompare(null); openDraft(row); }} />}
  </div></AppShell>;
}

function MatchTag({ state }: { state: MatchState }) {
  if (state.tone === "good") return <span className="hidden items-center gap-1 whitespace-nowrap text-xs text-[var(--muted)] md:inline-flex"><Check size={14} weight="bold" className="text-[var(--success)]" />{state.label}</span>;
  const tone = state.tone === "danger" ? "bg-[var(--danger-soft)] text-[var(--danger)]" : "bg-[var(--warning-soft)] text-[var(--warning)]";
  return <span className={`whitespace-nowrap rounded-full px-2.5 py-0.5 text-xs font-medium ${tone}`}>{state.label}</span>;
}

type ResolverProps = {
  row: Appointment;
  state: MatchState;
  picker: DirectoryPicker | null;
  draft: PersonDraft | null;
  onPick: (id: number, userId: number, fullName: string, positionTitle?: string) => void;
  onCompare: () => void;
  onPickerChange: (picker: DirectoryPicker | null) => void;
  onSearch: (appointmentId: number, query: string) => void;
  onDraftChange: (draft: PersonDraft | null) => void;
  onOpenDraft: () => void;
  onSubmitDraft: () => void;
};

function UnlinkedResolver({ row, state, picker, draft, onPick, onCompare, onPickerChange, onSearch, onDraftChange, onOpenDraft, onSubmitDraft }: ResolverProps) {
  const candidates = row.candidates ?? [];
  const pickerOpen = picker?.appointmentId === row.id;
  const draftOpen = draft?.appointmentId === row.id;

  return <div className="space-y-2.5">
    <p className="text-xs text-[var(--muted)]">{state.detail}{row.position ? ` · ในเอกสาร: ${row.position}` : ""}</p>
    {candidates.length > 0 && <div className="grid gap-2 sm:grid-cols-2">{candidates.map((candidate) => <button key={candidate.userId} type="button" onClick={() => onPick(row.id, candidate.userId, candidate.fullName, candidate.positionTitle)} className="focus-ring rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2.5 text-left transition hover:border-[var(--accent)] hover:bg-[var(--brand-blue-pale)]"><span className="block text-[13px] font-medium">{candidate.fullName}</span><span className="block text-xs text-[var(--muted)]">{candidate.positionTitle || "ไม่ระบุตำแหน่ง"} · ใกล้เคียง {Math.round(candidate.score * 100)}%</span><span className="mt-0.5 block text-xs font-medium text-[var(--accent)]">เลือกคนนี้</span></button>)}</div>}
    {pickerOpen && picker ? <div className="space-y-2 rounded-lg border border-[var(--accent)] bg-white p-3">
      <input autoFocus value={picker.query} onChange={(event) => onPickerChange({ ...picker, query: event.target.value })} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); onSearch(row.id, picker.query); } }} aria-label="ค้นหาบุคคลในระบบ" placeholder="พิมพ์ชื่อ อีเมล หน่วยงาน แล้วกด Enter" className={inputClass} />
      {picker.loading && <p className="text-xs text-[var(--muted)]">กำลังค้นหา...</p>}
      {!picker.loading && picker.results.slice(0, 8).map((user) => <button key={user.id} type="button" onClick={() => onPick(row.id, user.id, `${user.firstName} ${user.lastName}`.trim() || user.fullName, user.positionTitle)} className="focus-ring block w-full rounded-md border border-[var(--line)] px-2.5 py-1.5 text-left text-xs hover:border-[var(--accent)] hover:bg-[var(--brand-blue-pale)]"><span className="block font-medium">{user.fullName || `${user.firstName} ${user.lastName}`.trim()}</span><span className="block text-[var(--muted)]">{user.positionTitle || user.department || user.email || "-"}</span></button>)}
      {!picker.loading && picker.results.length === 0 && <p className="text-xs text-[var(--muted)]">กด Enter เพื่อค้นหา หรือสร้างบุคคลใหม่ถ้าไม่มีในระบบ</p>}
      <button type="button" onClick={() => onPickerChange(null)} className="focus-ring text-xs text-[var(--muted)] hover:text-[var(--ink)]">ปิด</button>
    </div> : draftOpen && draft ? <div className="space-y-2.5 rounded-lg border border-[var(--accent)] bg-white p-3">
      <p className="text-[13px] font-medium text-[var(--navy)]">สร้างบุคคลใหม่</p>
      {draft.conflictUserId && <div role="alert" className="flex flex-col gap-2 rounded-md bg-[var(--warning-soft)] px-3 py-2.5 sm:flex-row sm:items-center"><span className="flex-1 text-xs text-[var(--warning)]"><span className="font-semibold">มีบุคคลชื่อนี้ในระบบแล้ว</span> · ระบบไม่สร้างชื่อซ้ำ ถ้าเป็นคนเดียวกันให้ผูกกับบุคคลที่มีอยู่</span><button type="button" onClick={() => onPick(row.id, draft.conflictUserId ?? 0, `${draft.firstName} ${draft.lastName}`.trim(), draft.positionTitle)} className="focus-ring h-9 whitespace-nowrap rounded-md bg-[var(--accent)] px-3 text-xs font-medium text-white hover:bg-[var(--accent-strong)]">ผูกกับบุคคลนี้แทน</button></div>}
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="block"><span className="text-[11px] text-[var(--muted)]">ชื่อ</span><input value={draft.firstName} onChange={(event) => onDraftChange({ ...draft, firstName: event.target.value, conflictUserId: undefined })} aria-invalid={Boolean(draft.conflictUserId)} className={`${inputClass} mt-1 ${draft.conflictUserId ? "border-[var(--warning-dot)] bg-[#fffbf0]" : ""}`} /></label>
        <label className="block"><span className="text-[11px] text-[var(--muted)]">นามสกุล</span><input value={draft.lastName} onChange={(event) => onDraftChange({ ...draft, lastName: event.target.value, conflictUserId: undefined })} aria-invalid={Boolean(draft.conflictUserId)} className={`${inputClass} mt-1 ${draft.conflictUserId ? "border-[var(--warning-dot)] bg-[#fffbf0]" : ""}`} /></label>
        <label className="block"><span className="text-[11px] text-[var(--muted)]">ตำแหน่ง</span><input value={draft.positionTitle} onChange={(event) => onDraftChange({ ...draft, positionTitle: event.target.value })} className={`${inputClass} mt-1`} /></label>
        <label className="block"><span className="text-[11px] text-[var(--muted)]">อีเมล (ใช้แจ้งเตือนหมดวาระ)</span><input value={draft.email} onChange={(event) => onDraftChange({ ...draft, email: event.target.value })} placeholder="name@kku.ac.th" className={`${inputClass} mt-1`} /></label>
      </div>
      <div className="flex gap-2"><button type="button" disabled={draft.saving || !draft.firstName.trim() || !draft.lastName.trim() || Boolean(draft.conflictUserId)} onClick={onSubmitDraft} className="focus-ring h-9 whitespace-nowrap rounded-md bg-[var(--accent)] px-3.5 text-xs font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{draft.saving ? "กำลังบันทึก..." : "บันทึกและผูก"}</button><button type="button" onClick={() => onDraftChange(null)} className="focus-ring h-9 rounded-md px-3 text-xs text-[var(--muted)] hover:text-[var(--ink)]">ยกเลิก</button></div>
    </div> : <div className="flex flex-wrap items-center gap-2">
      {candidates.length > 1 && <button type="button" onClick={onCompare} className={`${smallButton} border-[var(--accent)] bg-white font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]`}><UsersThree size={14} /> เปรียบเทียบข้อมูลทั้ง {candidates.length} คน</button>}
      <button type="button" onClick={() => onPickerChange({ appointmentId: row.id, query: row.fullName, results: [], loading: false })} className={`${smallButton} border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--line-soft)]`}><MagnifyingGlass size={14} /> เลือกจากรายชื่อ</button>
      <button type="button" onClick={onOpenDraft} className={`${smallButton} border-[var(--line-strong)] bg-white hover:bg-[var(--line-soft)]`}><UserPlus size={14} /> สร้างบุคคลใหม่</button>
    </div>}
  </div>;
}

type CompareDialogProps = {
  compare: CandidateCompare;
  onSelect: (userId: number) => void;
  onClose: () => void;
  onConfirm: (candidate: DirectoryCandidate) => void;
  onCreate: () => void;
};

function positionMatches(documentPosition: string, user?: DirectoryUser) {
  const target = documentPosition.trim();
  if (!target || !user) return false;
  return [user.positionTitle, user.jobTitle].some((value) => value && (target.includes(value) || value.includes(target)));
}

function CandidateCompareDialog({ compare, onSelect, onClose, onConfirm, onCreate }: CompareDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const { appointment, users, selected } = compare;
  const candidates = appointment.candidates ?? [];

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog && !dialog.open) dialog.showModal();
  }, []);

  const selectedCandidate = candidates.find((candidate) => candidate.userId === selected);

  return <dialog ref={dialogRef} aria-labelledby="compare-title" onCancel={(event) => { event.preventDefault(); onClose(); }} className="m-auto w-[min(94vw,760px)] rounded-xl bg-white p-0 text-[var(--ink)] shadow-[0_20px_48px_rgba(0,0,0,0.3)] backdrop:bg-[rgba(3,20,69,0.62)]">
    <div className="flex items-start gap-3.5 border-b border-[var(--line)] px-6 pb-4 pt-6 sm:px-7">
      <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-[10px] bg-[var(--danger-soft)] text-[var(--danger)]"><UsersThree size={22} /></span>
      <div className="flex-1"><h2 id="compare-title" className="text-xl font-semibold text-[var(--navy)]">มีชื่อซ้ำกันในระบบ {candidates.length} คน</h2><p className="mt-1 text-sm font-light leading-6 text-[var(--muted)]">เลือกบุคคลที่ตรงกับรายชื่อในเอกสาร เมื่อบันทึกแล้ว ครั้งต่อไประบบจะผูกชื่อนี้ให้อัตโนมัติ</p></div>
    </div>
    <div className="space-y-4 px-6 py-5 sm:px-7">
      <div className="rounded-lg bg-[var(--canvas)] px-4 py-3"><p className="text-[11px] text-[var(--muted)]">จากเอกสาร</p><p className="mt-0.5 font-medium">{appointment.fullName}</p><p className="text-[13px] font-light text-[var(--muted)]">{[appointment.position, appointment.committeeRole, appointment.pageNo ? `หน้า ${appointment.pageNo}` : ""].filter(Boolean).join(" · ")}</p></div>
      <fieldset className="space-y-3"><legend className="mb-2.5 text-[13px] font-medium text-[var(--navy)]">บุคคลในระบบที่ชื่อตรงกัน</legend>
        {candidates.map((candidate) => {
          const user = users?.find((entry) => entry.id === candidate.userId);
          const active = candidate.userId === selected;
          const details = [["ตำแหน่ง", user?.positionTitle || user?.jobTitle || candidate.positionTitle], ["หน่วยงาน", user?.department || user?.faculty], ["อีเมล", user?.email], ["รหัสบุคลากร", user?.userId || user?.externalId]];
          return <label key={candidate.userId} className={`flex cursor-pointer gap-3.5 rounded-[10px] p-4 transition ${active ? "border-2 border-[var(--accent)] bg-[var(--brand-blue-pale)]" : "border border-[var(--line-strong)] hover:border-[var(--accent)]"}`}>
            <input type="radio" name="compare-candidate" checked={active} onChange={() => onSelect(candidate.userId)} className="mt-1 h-[18px] w-[18px] accent-[var(--accent)]" />
            <span className="min-w-0 flex-1">
              <span className="flex flex-wrap items-center gap-2"><span className="font-semibold">{candidate.fullName}</span>{user?.source && <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium ${user.source === "MANUAL" ? "bg-[var(--warning-soft)] text-[var(--warning)]" : "bg-[var(--accent-soft)] text-[#155e97]"}`}>{user.source === "MANUAL" ? "เพิ่มโดยเจ้าหน้าที่" : "จาก sync"}</span>}{positionMatches(appointment.position, user) && <span className="rounded-full bg-[var(--success-soft)] px-2 py-0.5 text-[11px] font-medium text-[var(--success)]">ตำแหน่งตรงกับเอกสาร</span>}</span>
              <span className="mt-2 grid gap-x-5 gap-y-1.5 sm:grid-cols-2">{details.map(([label, value]) => <span key={label} className="block"><span className="block text-[11px] text-[var(--muted)]">{label}</span><span className={`block text-[13px] ${value ? "" : "text-[var(--subtle)]"}`}>{users === null ? "กำลังโหลด..." : value || "ไม่ระบุ"}</span></span>)}</span>
            </span>
          </label>;
        })}
      </fieldset>
      <p className="text-[13px] text-[var(--muted)]">ไม่ใช่ทั้ง {candidates.length} คน? <button type="button" onClick={onCreate} className="focus-ring text-[var(--accent)] hover:underline">สร้างบุคคลใหม่</button></p>
    </div>
    <div className="flex justify-end gap-2.5 border-t border-[var(--line)] bg-[var(--surface-muted)] px-6 py-4 sm:px-7">
      <button type="button" onClick={onClose} className="focus-ring h-11 rounded-md px-4 text-sm text-[var(--muted)] hover:bg-[var(--line-soft)]">ยกเลิก</button>
      <button type="button" disabled={!selectedCandidate} onClick={() => selectedCandidate && onConfirm(selectedCandidate)} className="focus-ring inline-flex h-11 items-center gap-2 rounded-md bg-[var(--accent)] px-5 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:opacity-50"><Check size={16} weight="bold" />ผูกกับบุคคลนี้</button>
    </div>
  </dialog>;
}
