"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { ChangeEvent, Fragment, KeyboardEvent, useEffect, useMemo, useRef, useState } from "react";
import { ArrowCounterClockwise, CaretLeft, CaretRight, Check, CheckCircle, Info, MagnifyingGlass, MapPin, Trash, UploadSimple, UserPlus, UsersThree, WarningCircle } from "@phosphor-icons/react";
import { AppShell, StatusBadge } from "@/components/app-shell";
import { useMe } from "@/components/me-context";
import { useToast } from "@/components/toast";
import { DepartmentDutiesPanel } from "@/components/department-duties-panel";
import { ApiDocument, ApiError, Appointment, DirectoryCandidate, ExtractedField, DirectoryUser, DocumentRevision, cancelDocument, confirmDocument, returnDocument, createDirectoryUser, deleteDocument, formatFileSize, formatThaiDate, getDirectoryUsers, getDocument, getDocumentPageImageUrl, getDocumentRevisions, getOCRSettings, reprocessDocument, replaceDocumentFile, statusLabel, updateDocument } from "@/lib/api";
import { MatchState, departmentOf, isLinked, matchState, personLabel, rosterRows, summarizeDepartments } from "@/lib/member-review";
import { can } from "@/lib/permissions";
import { canRescan, processingNote, rescanNeedsConfirm } from "@/lib/processing-status";

type PanelView = "original" | "roster" | "members" | "metadata" | "duties" | "history";

// บอกว่าระบบอ่านวันที่ในช่องนี้ได้ว่าอะไร ถ้าอ่านไม่ออกต้องแก้ก่อนกดยืนยัน
// ค่าที่แก้แล้วยังไม่บันทึกจะถูกตรวจตอนบันทึก เพราะตัวอ่านวันที่ไทยอยู่ที่ระบบหลังบ้าน
function dateReading(field: ExtractedField, saved?: ExtractedField): { ok: boolean; text: string } | null {
  if (!field.dateStatus) return null;
  if (!saved || saved.value !== field.value) return { ok: true, text: "ระบบจะอ่านวันที่ให้ตอนกดบันทึก" };
  if (field.dateStatus === "read" && field.readDate) return { ok: true, text: `อ่านได้ว่า ${formatThaiDate(field.readDate)}` };
  if (field.dateStatus === "fromNow") return { ok: true, text: "มีผลตั้งแต่วันที่ออกคำสั่ง" };
  if (field.dateStatus === "unreadable") return { ok: false, text: "อ่านวันที่ไม่ได้ · พิมพ์ใหม่ เช่น 11 ตุลาคม 2569 แล้วบันทึก" };
  return null;
}

const metadataKeys = ["order_type", "issuing_unit", "order_no", "order_year_be", "committee_name", "issued_date", "effective_date", "expiry_date", "signer_name", "signer_position"];

const panelTabs: { value: PanelView; label: string }[] = [
  { value: "original", label: "ต้นฉบับเอกสาร" },
  { value: "roster", label: "รายชื่อทั้งหมด" },
  { value: "members", label: "ตรวจรายชื่อ" },
  { value: "metadata", label: "ข้อมูลคำสั่ง" },
  { value: "duties", label: "หน้าที่รับผิดชอบ" },
  { value: "history", label: "ประวัติการแก้ไข" },
];

// ป้ายบอกว่าใครเป็นคนผูกชื่อนี้ ผู้ตรวจจะเห็นทันทีว่าแถวไหนระบบผูกให้เอง
const linkedBy: Record<string, string> = { EXACT: "ระบบผูกให้ · ชื่อตรง", ALIAS: "ระบบผูกให้ · เคยผูกไว้", POSITION: "ระบบผูกให้ · ตามตำแหน่ง", MANUAL: "เลือกเอง" };

// ชื่อที่แค่ใกล้เคียง ระบบเลือกคนที่เดาไว้ให้ ถ้าถูกต้องก็ไม่ต้องกดอะไร บันทึกแล้วผูกให้เลย
function guessFor(row: Appointment) {
  return !isLinked(row) && row.nameMatchMethod === "FUZZY" && row.candidates?.length ? row.candidates[0] : undefined;
}

type RestorePoint = { fullName: string; position: string; nameMatchMethod: string; nameMatchScore: number };
type DirectoryPicker = { appointmentId: number; query: string; results: DirectoryUser[]; loading: boolean };
type PersonDraft = { appointmentId: number; firstName: string; lastName: string; positionTitle: string; email: string; saving: boolean; conflictUserId?: number };
type CandidateCompare = { appointment: Appointment; users: DirectoryUser[] | null; selected: number };

const inputClass = "focus-ring h-9 w-full rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-[13px] outline-none focus:border-[var(--accent)]";
const smallButton = "focus-ring inline-flex h-9 items-center gap-1.5 whitespace-nowrap rounded-md border px-3 text-xs transition";

export function DocumentWorkspace({ document: initialDocument }: { document: ApiDocument }) {
  const router = useRouter();
  const toast = useToast();
  const { me } = useMe();
  const [document, setDocument] = useState(initialDocument);
  const [fields, setFields] = useState(initialDocument.fields ?? []);
  const [appointments, setAppointments] = useState(initialDocument.appointments ?? []);
  const [revisions, setRevisions] = useState<DocumentRevision[]>([]);
  const [selectedPage, setSelectedPage] = useState(1);
  const [selectedAppointmentId, setSelectedAppointmentId] = useState<number | null>(null);
  const [panel, setPanel] = useState<PanelView>("original");
  const tabRefs = useRef<Partial<Record<PanelView, HTMLButtonElement | null>>>({});
  const panelRef = useRef<HTMLDivElement>(null);
  const [onlyUnlinked, setOnlyUnlinked] = useState(false);
  const [memberQuery, setMemberQuery] = useState("");
  const [rejectedGuesses, setRejectedGuesses] = useState<Set<number>>(new Set());
  const [openRows, setOpenRows] = useState<Record<number, boolean>>({});
  const [saving, setSaving] = useState(false);
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelReason, setCancelReason] = useState("");
  const [cancelling, setCancelling] = useState(false);
  const [cancelError, setCancelError] = useState("");
  const cancelled = document.status === "CANCELLED";
  const editable = !cancelled && !cancelling && can(me?.role ?? "", "edit");
  const cancelReasonRef = useRef<HTMLTextAreaElement>(null);
  const [deleteArmed, setDeleteArmed] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [picker, setPicker] = useState<DirectoryPicker | null>(null);
  const [draft, setDraft] = useState<PersonDraft | null>(null);
  const [compare, setCompare] = useState<CandidateCompare | null>(null);
  const [restorePoints, setRestorePoints] = useState<Record<number, RestorePoint>>({});
  const [replaceArmed, setReplaceArmed] = useState(false);
  const [replacing, setReplacing] = useState(false);
  const [rescanning, setRescanning] = useState(false);
  const [rescanArmed, setRescanArmed] = useState(false);
  // เกณฑ์เดียวกับที่ backend ใช้ตัดสินว่าเอกสารต้องตรวจสอบ ปรับได้ที่หน้าตั้งค่า OCR
  const [reviewThreshold, setReviewThreshold] = useState(0.9);
  const replaceInputRef = useRef<HTMLInputElement>(null);

  const selectedOCRPage = document.ocrPages?.find((page) => page.pageNo === selectedPage);
  const pageWidth = selectedOCRPage?.imageWidth || 1;
  const pageHeight = selectedOCRPage?.imageHeight || 1;
  const metadataFields = fields.filter((field) => metadataKeys.includes(field.fieldKey));
  const lowConfidenceCount = useMemo(() => fields.filter((field) => field.confidence < reviewThreshold || !field.value).length + appointments.filter((item) => item.confidence < reviewThreshold || item.nameMatchMethod !== "EXACT").length, [fields, appointments, reviewThreshold]);

  const departmentSummary = useMemo(() => summarizeDepartments(appointments), [appointments]);
  const unlinkedAppointments = useMemo(() => appointments.filter((row) => !isLinked(row) && !(guessFor(row) && !rejectedGuesses.has(row.id))), [appointments, rejectedGuesses]);
  const linkedCount = appointments.length - unlinkedAppointments.length;
  const linkedByPosition = appointments.filter((row) => isLinked(row) && row.nameMatchMethod === "POSITION").length;
  const pendingGuesses = appointments.filter((row) => guessFor(row) && !rejectedGuesses.has(row.id));
  const missingHolders = unlinkedAppointments.filter((row) => !row.fullName.trim()).length;
  const missingPeople = unlinkedAppointments.filter((row) => row.fullName.trim() && !row.candidates?.length).length;
  const memberRows = useMemo(() => {
    const query = memberQuery.trim().toLowerCase();
    return appointments.map((row, index) => ({ row, no: index + 1 })).filter(({ row }) => (!onlyUnlinked || unlinkedAppointments.includes(row)) && (!query || [row.fullName, row.position, row.committeeRole, row.department].join(" ").toLowerCase().includes(query)));
  }, [appointments, onlyUnlinked, unlinkedAppointments, memberQuery]);
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
    getOCRSettings().then((response) => setReviewThreshold(response.data.reviewThreshold)).catch(() => setReviewThreshold(0.9));
  }, []);

  useEffect(() => {
    getDocumentRevisions(document.id).then((response) => setRevisions(response.data)).catch(() => setRevisions([]));
  }, [document.id]);

  useEffect(() => {
    if (cancelled || !["QUEUED", "PREPARING", "EXTRACTING", "RETRY_WAIT"].includes(document.processingStage ?? "")) return;
    let active = true;
    const timer = window.setInterval(() => {
      getDocument(document.id).then(({ data }) => {
        if (!active) return;
        setDocument((current) => current.status === "CANCELLED" ? current : data);
        if (data.processingStage === "EXTRACTED" || data.processingStage === "FAILED") {
          setFields(data.fields ?? []);
          setAppointments(data.appointments ?? []);
        }
      }).catch(() => {});
    }, 2000);
    return () => { active = false; window.clearInterval(timer); };
  }, [document.id, document.processingStage, cancelled]);

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

  const [sendingBack, setSendingBack] = useState(false);
  // ส่งเอกสารที่ยืนยันแล้วกลับไป "รอตรวจสอบ" เพื่อแก้ไข แล้วยืนยันใหม่อีกครั้ง
  const sendBack = async () => {
    setSendingBack(true);
    try {
      const response = await returnDocument(document.id);
      setDocument(response.data);
      toast("ส่งกลับไปตรวจสอบแล้ว · แก้ไขเสร็จกดยืนยันอีกครั้ง");
    } catch (reason) {
      showError(reason, "ส่งกลับไปตรวจสอบไม่สำเร็จ");
    } finally {
      setSendingBack(false);
    }
  };

  const rejectGuess = (id: number) => setRejectedGuesses((current) => new Set(current).add(id));
  const acceptAllGuesses = () => {
    for (const row of pendingGuesses) {
      const guess = guessFor(row);
      if (guess) linkAppointment(row.id, guess.userId, guess.fullName, guess.positionTitle);
    }
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

  const locateAppointment = (id: number, pageNo: number) => {
    setSelectedAppointmentId(id);
    setSelectedPage(Math.min(document.pageCount || Math.max(1, pageNo), Math.max(1, pageNo || 1)));
    setPanel("original");
    window.requestAnimationFrame(() => {
      panelRef.current?.focus();
      panelRef.current?.scrollIntoView({ block: "start" });
    });
  };

  const onTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, value: PanelView) => {
    const index = panelTabs.findIndex((tab) => tab.value === value);
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % panelTabs.length;
    else if (event.key === "ArrowLeft") next = (index - 1 + panelTabs.length) % panelTabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = panelTabs.length - 1;
    else return;
    event.preventDefault();
    const selected = panelTabs[next].value;
    setPanel(selected);
    tabRefs.current[selected]?.focus();
    tabRefs.current[selected]?.scrollIntoView({ block: "nearest", inline: "nearest" });
  };
  const toggleRow = (id: number, open: boolean) => setOpenRows((rows) => ({ ...rows, [id]: !open }));
  const showUnlinked = () => { setPanel("members"); setOnlyUnlinked(true); setMemberQuery(""); };

  const buildPayload = () => ({
    responsibilities: document.responsibilities,
    additionalReferences: document.additionalReferences,
    fields: fields.map((field) => ({ id: field.id, fieldKey: field.fieldKey, fieldLabel: field.fieldLabel, value: field.value, verified: field.verified })),
    appointments: appointments.map((item) => ({ id: item.id, directoryUserId: item.directoryUserId ?? (rejectedGuesses.has(item.id) ? undefined : guessFor(item)?.userId), fullName: item.fullName, position: item.position, department: item.department, committeeRole: item.committeeRole, responsibilities: item.responsibilities, extraDuties: item.extraDuties ?? "", confidence: item.confidence, pageNo: item.pageNo, boundingBox: item.boundingBox, verified: item.verified })),
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

  const onRescan = async () => {
    if (rescanNeedsConfirm(document) && !rescanArmed) {
      setRescanArmed(true);
      return;
    }
    setRescanning(true);
    try {
      await reprocessDocument(document.id);
      window.location.reload();
    } catch (reason) {
      showError(reason, "สแกนไฟล์เดิมใหม่ไม่สำเร็จ");
      setRescanning(false);
      setRescanArmed(false);
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

  useEffect(() => {
    if (cancelOpen) cancelReasonRef.current?.focus();
  }, [cancelOpen]);

  const submitCancellation = async () => {
    const reason = cancelReason.trim();
    if (!reason || Array.from(reason).length > 2000) {
      setCancelError("กรุณาระบุเหตุผลที่ยกเลิก 1–2,000 ตัวอักษร");
      cancelReasonRef.current?.focus();
      return;
    }
    setCancelling(true);
    setCancelError("");
    try {
      const { data } = await cancelDocument(document.id, reason);
      setDocument(data);
      setFields(data.fields ?? []);
      setAppointments(data.appointments ?? []);
      setCancelOpen(false);
      setPicker(null); setDraft(null); setCompare(null); setReplaceArmed(false);
      toast("ยกเลิกเอกสารเรียบร้อยแล้ว");
      getDocumentRevisions(document.id).then(({ data }) => setRevisions(data)).catch(() => {});
      router.refresh();
    } catch (reason) {
      setCancelError(reason instanceof Error ? reason.message : "ไม่สามารถยกเลิกเอกสารได้");
    } finally { setCancelling(false); }
  };

  const bandMeta = <>
    <StatusBadge status={statusLabel(document.status)} />
    {document.status === "CONFIRMED" && can(me?.role ?? "", "cancel") && <button type="button" disabled={sendingBack} onClick={sendBack} className="focus-ring inline-flex h-10 items-center gap-1.5 rounded-md border border-white/45 px-3.5 text-[13px] font-medium text-white hover:bg-white/10 disabled:opacity-50"><ArrowCounterClockwise size={15} />{sendingBack ? "กำลังส่งกลับ..." : "ส่งกลับไปตรวจสอบ"}</button>}
    {!cancelled && can(me?.role ?? "", "cancel") && <button type="button" disabled={saving || replacing || rescanning || cancelling} onClick={() => { setCancelOpen(true); setCancelError(""); }} className="focus-ring inline-flex h-10 items-center gap-1.5 rounded-md border border-white/45 px-3.5 text-[13px] font-medium text-white hover:bg-white/10 disabled:opacity-50"><WarningCircle size={15} />ยกเลิกเอกสาร</button>}
    {!cancelled && can(me?.role ?? "", "import") && canRescan(document) && <button type="button" disabled={saving || replacing || rescanning || cancelling} onClick={onRescan} className={`focus-ring inline-flex h-10 items-center gap-1.5 rounded-md px-3.5 text-[13px] font-medium transition disabled:opacity-50 ${rescanArmed ? "bg-[var(--danger)] text-white" : "border border-white/45 text-white hover:bg-white/10"}`}><ArrowCounterClockwise size={15} className={rescanning ? "animate-spin" : ""} />{rescanning ? "กำลังส่งเข้าคิว..." : rescanArmed ? "ยืนยันการสแกนใหม่" : rescanNeedsConfirm(document) ? "สแกนไฟล์เดิมใหม่" : "ประมวลผลใหม่"}</button>}
    {rescanArmed && !rescanning && <button type="button" onClick={() => setRescanArmed(false)} className="focus-ring h-10 rounded-md px-2 text-[13px] text-[var(--band-text)] hover:text-white">ยกเลิก</button>}
    <span className="text-[13px] text-[var(--band-text)]">นำเข้า {formatThaiDate(document.createdAt)}</span>
    {can(me?.role ?? "", "delete") && <button type="button" onClick={removeDocument} disabled={deleting} className={`focus-ring inline-flex h-10 items-center gap-1.5 rounded-md px-3.5 text-[13px] font-medium transition ${deleteArmed ? "bg-[var(--danger)] text-white" : "border border-white/45 text-white hover:bg-white/10"}`}><Trash size={15} />{deleting ? "กำลังลบ..." : deleteArmed ? "ยืนยันการลบ" : "ลบข้อมูลทดสอบ"}</button>}
    {deleteArmed && !deleting && <button type="button" onClick={() => setDeleteArmed(false)} className="focus-ring h-10 rounded-md px-2 text-[13px] text-[var(--band-text)] hover:text-white">ยกเลิก</button>}
  </>;

  return <AppShell backHref="/documents" breadcrumb="เอกสารทั้งหมด / ตรวจสอบข้อมูล" title={document.title || document.originalFilename} meta={bandMeta}><div className="space-y-6">
    {cancelled && <section role="status" className="rounded-lg border border-[var(--danger)]/25 bg-[var(--danger-soft)] p-5">
      <h2 className="flex items-center gap-2 text-base font-semibold text-[var(--danger)]"><WarningCircle size={20} />ยกเลิกเอกสารแล้ว</h2>
      <p className="mt-3 text-xs font-medium text-[var(--muted)]">เหตุผลที่ยกเลิก</p>
      <p className="mt-1 whitespace-pre-wrap break-words text-sm leading-6">{document.cancellationReason}</p>
      <p className="mt-3 break-words text-xs text-[var(--muted)]">ผู้ยกเลิก: {document.cancelledBy || "เจ้าหน้าที่"} · {document.cancelledAt ? new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: "Asia/Bangkok" }).format(new Date(document.cancelledAt)) : "ไม่ระบุเวลา"}</p>
      <p className="mt-2 text-xs text-[var(--muted)]">เก็บต้นฉบับและประวัติไว้สำหรับตรวจสอบย้อนหลัง เอกสารนี้ไม่รวมในรายงานภาระงานและการแจ้งเตือนหมดอายุ</p>
    </section>}
    {rescanArmed && !cancelled && <p role="alert" className="flex items-start gap-2 rounded-lg border border-[var(--danger)]/25 bg-[var(--danger-soft)] px-4 py-3 text-sm leading-6 text-[var(--danger)]"><WarningCircle size={18} weight="fill" className="mt-1 shrink-0" />ข้อมูลที่ตรวจและแก้ไขไว้จะถูกแทนด้วยผลสแกนใหม่ และต้องยืนยันเอกสารใหม่อีกครั้ง</p>}
    {cancelOpen && !cancelled && <form onSubmit={(event) => { event.preventDefault(); void submitCancellation(); }} aria-labelledby="cancel-document-title" className="card space-y-4 border border-[var(--danger)]/25 p-5">
      <div><h2 id="cancel-document-title" className="text-base font-semibold text-[var(--navy)]">ยกเลิกเอกสาร</h2><p className="mt-1 text-sm text-[var(--muted)]">หลังยกเลิกจะเปิดดูย้อนหลังได้ แต่ไม่สามารถแก้ไขหรือยืนยันข้อมูลได้ ข้อมูลที่ยังไม่ได้บันทึกจะไม่ถูกเก็บ</p></div>
      <label htmlFor="cancel-document-reason" className="block text-sm font-medium">เหตุผลที่ยกเลิก <span className="text-[var(--danger)]">*</span></label>
      <textarea ref={cancelReasonRef} id="cancel-document-reason" required maxLength={4000} disabled={cancelling} value={cancelReason} onChange={(event) => setCancelReason(event.target.value)} rows={3} aria-invalid={Boolean(cancelError)} aria-describedby="cancel-document-help cancel-document-error" className="focus-ring w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6" />
      <p id="cancel-document-help" className="text-xs text-[var(--muted)]">ระบุได้ไม่เกิน 2,000 ตัวอักษร · {Array.from(cancelReason.trim()).length.toLocaleString("th-TH")} / 2,000</p>
      <p id="cancel-document-error" role={cancelError ? "alert" : undefined} className="text-sm text-[var(--danger)]">{cancelError}</p>
      <div className="flex flex-wrap justify-end gap-2"><button type="button" disabled={cancelling} onClick={() => setCancelOpen(false)} className="focus-ring h-11 rounded-md border border-[var(--line)] px-4 text-sm">กลับไปตรวจสอบ</button><button type="submit" disabled={cancelling || !cancelReason.trim()} className="focus-ring h-11 rounded-md bg-[var(--danger)] px-4 text-sm font-medium text-white disabled:opacity-50">{cancelling ? "กำลังยกเลิก..." : "ยืนยันยกเลิกเอกสาร"}</button></div>
    </form>}
    <section className="card overflow-hidden">
        <div className="space-y-3 px-5 pb-4 pt-5 lg:px-6">
          {!cancelled && processingNote(document) && <div role="alert" className="flex flex-col gap-3 rounded-lg border border-[var(--warning)]/30 bg-[var(--warning-soft)] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
<div className="flex min-w-0 items-start gap-2">
<WarningCircle size={18} className="mt-0.5 shrink-0 text-[var(--warning)]" />
<div className="min-w-0">
<p className="text-sm font-medium text-[var(--navy)]">{processingNote(document)}</p>{can(me?.role ?? "", "settings") && document.ocrError && <p className="mt-1 break-words text-xs leading-5 text-[var(--muted)]">สาเหตุ: {document.ocrError}</p>}</div>
</div></div>}
          {!cancelled && (document.processingStage === "QUEUED" || document.processingStage === "PREPARING" || document.processingStage === "EXTRACTING") && <p role="status" className="rounded-lg bg-[var(--accent-soft)] px-4 py-3 text-sm text-[var(--navy)]">กำลังประมวลผลเอกสาร ระบบจะอัปเดตข้อมูลให้อัตโนมัติ</p>}
          <div role="status" className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <div className="min-w-[180px] flex-1"><p className="text-xs text-[var(--muted)]">ผูกกับบุคคลแล้ว <span className="font-medium text-[var(--ink)]">{linkedCount}</span> จาก {appointments.length} รายชื่อ</p><span className="mt-1.5 block h-1 overflow-hidden rounded-full bg-[var(--line)]"><span className="block h-full rounded-full bg-[var(--accent)] transition-all" style={{ width: `${appointments.length ? Math.round((linkedCount / appointments.length) * 100) : 0}%` }} /></span></div>
            {unlinkedAppointments.length > 0 && <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs"><span className="h-2 w-2 rounded-full bg-[var(--highlight)]" />ยังไม่ผูก {unlinkedAppointments.length}</span>}
            {lowConfidenceCount > 0 && <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs"><span className="h-2 w-2 rounded-full bg-[var(--warning-dot)]" />ควรตรวจ {lowConfidenceCount}</span>}
            {unlinkedAppointments.length > 0 && <button type="button" onClick={showUnlinked} className="focus-ring h-8 whitespace-nowrap rounded text-xs font-medium text-[var(--accent)] hover:underline">ดูชื่อที่ยังไม่ผูก →</button>}
          </div>
        </div>
      <div role="tablist" aria-label="ส่วนของเอกสาร" aria-orientation="horizontal" className="shell-scrollbar flex overflow-x-auto border-t border-[var(--line)] px-3">{panelTabs.map((tab) => <button key={tab.value} ref={(button) => { tabRefs.current[tab.value] = button; }} id={`document-tab-${tab.value}`} type="button" role="tab" aria-selected={panel === tab.value} aria-controls="document-tab-panel" tabIndex={panel === tab.value ? 0 : -1} onClick={() => setPanel(tab.value)} onKeyDown={(event) => onTabKeyDown(event, tab.value)} className={`focus-ring flex h-14 shrink-0 items-center gap-2 whitespace-nowrap px-4 text-sm transition ${panel === tab.value ? "font-medium text-[var(--accent)] shadow-[inset_0_-3px_0_var(--accent)]" : "text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--ink)]"}`}>{tab.label}{(tab.value === "roster" || tab.value === "members" || tab.value === "history") && <span className={`rounded-full px-2 py-0.5 text-[11px] ${panel === tab.value ? "bg-[var(--accent-soft)]" : "bg-[var(--line-soft)]"}`}>{tab.value === "history" ? revisions.length : appointments.length}</span>}</button>)}</div>
    </section>
    <div ref={panelRef} id="document-tab-panel" role="tabpanel" aria-labelledby={`document-tab-${panel}`} tabIndex={0} className="focus-ring min-w-0 scroll-mt-5 rounded-lg">
      {panel === "original" && (
      <section className="flex min-h-[420px] flex-col overflow-hidden rounded-[10px] bg-[#e9eef3] min-h-[560px] shadow-[var(--shadow-card)]">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--line)] bg-white py-3 pl-5 pr-3"><div><p className="text-sm font-semibold text-[var(--navy)]">ต้นฉบับเอกสาร</p><p className="mt-0.5 text-xs font-light text-[var(--muted)]">{document.pageCount || "-"} หน้า · {formatFileSize(document.fileSizeBytes)} · {document.ocrProvider || "นำเข้าข้อมูลโดยตรง"}</p></div><div className="flex items-center gap-1"><button type="button" disabled={selectedPage <= 1} onClick={() => setSelectedPage((page) => Math.max(1, page - 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)] disabled:cursor-not-allowed disabled:opacity-40" aria-label="หน้าก่อนหน้า"><CaretLeft size={17} /></button><span className="px-1.5 text-[13px]">หน้า {selectedPage} / {document.pageCount || 1}</span><button type="button" disabled={selectedPage >= (document.pageCount || 1)} onClick={() => setSelectedPage((page) => Math.min(document.pageCount || 1, page + 1))} className="focus-ring rounded-md p-2 text-[var(--muted)] hover:bg-[var(--line-soft)] disabled:cursor-not-allowed disabled:opacity-40" aria-label="หน้าถัดไป"><CaretRight size={17} /></button></div></div>
        <div className="shell-scrollbar flex flex-1 items-start justify-center overflow-auto p-6">{document.sourceType.toLowerCase() === "pdf" ? <div className="relative w-full max-w-[900px] overflow-hidden bg-white shadow-[0_8px_28px_rgba(17,40,62,0.14)]"><img src={getDocumentPageImageUrl(document.id, selectedPage)} alt={`หน้า ${selectedPage} ของ ${document.title}`} className="block h-auto w-full" /><div className="pointer-events-none absolute inset-0">{(selectedOCRPage?.lines ?? []).map((line) => { const box = parseBox(line.boundingBox); return box ? <span key={line.id} title={line.text} className="absolute border border-sky-400/50 bg-sky-300/5" style={box} /> : null; })}{appointments.filter((item) => item.pageNo === selectedPage).map((item) => { const box = parseBox(item.boundingBox); return box ? <span key={`appointment-${item.id}`} title={item.fullName} className={`absolute ${selectedAppointmentId === item.id ? "z-10 border-2 border-dashed border-[var(--highlight)] bg-[rgba(240,72,20,0.1)]" : "border-2 border-amber-500 bg-amber-300/20"}`} style={box} /> : null; })}{fields.filter((field) => field.pageNo === selectedPage).map((field) => { const box = parseBox(field.boundingBox); return box ? <span key={`field-${field.id}`} title={field.fieldLabel} className="absolute border-2 border-emerald-500 bg-emerald-300/20" style={box} /> : null; })}</div></div> : <div className="min-h-[660px] w-full max-w-[900px] whitespace-pre-wrap bg-white p-7 font-mono text-xs leading-6 shadow-[0_8px_28px_rgba(17,40,62,0.14)]">{document.ocrText || "กำลังเตรียมตัวอย่างข้อมูล..."}</div>}</div>
        <div className="border-t border-[var(--line)] bg-white px-5 py-2.5 text-[11px] text-[var(--muted)]">กรอบฟ้า = OCR · กรอบเหลือง = รายชื่อ · กรอบเขียว = ฟิลด์สกัด · กรอบส้มเส้นประ = รายการที่เลือก</div>
      </section>
      )}
      {panel !== "original" && panel !== "history" && <section className="card flex min-w-0 flex-col overflow-hidden">
        <div className="border-b border-[var(--line)] px-5 py-4 lg:px-6"><h2 className="text-base font-semibold text-[var(--navy)]">{panelTabs.find((tab) => tab.value === panel)?.label}</h2></div>
        {panel === "roster" && <div className="flex-1">
          <div className="flex items-baseline justify-between px-5 py-3.5 lg:px-6"><p className="text-sm font-semibold text-[var(--navy)]">รายชื่อทั้งหมดในคำสั่ง</p><p className="text-xs text-[var(--muted)]">{appointments.length} รายชื่อ · {departmentSummary.length} ฝ่าย</p></div>
          <div className="overflow-x-auto"><table className="data-grid w-full min-w-[680px] text-left text-sm"><thead><tr><th className="w-12 px-5 py-2.5 lg:pl-6">#</th><th className="px-3 py-2.5">ชื่อ</th><th className="px-3 py-2.5">ฝ่าย</th><th className="px-3 py-2.5">ตำแหน่ง</th><th className="px-5 py-2.5 lg:pr-6">บทบาท</th></tr></thead><tbody>{roster.map((row) => <tr key={row.id}><td className="px-5 py-3 text-xs text-[var(--subtle)] lg:pl-6">{row.no}</td><td className="px-3 py-3 font-medium"><span className="inline-flex items-center gap-2">{row.unlinked && <span aria-label="ยังไม่ผูกกับบุคคล" className="h-2 w-2 shrink-0 rounded-full bg-[#f0882a]" />}{row.fullName || "-"}</span></td><td className="px-3 py-3">{row.department}</td><td className="px-3 py-3 text-[var(--muted)]">{row.position || "-"}</td><td className="px-5 py-3 lg:pr-6">{row.role || "-"}</td></tr>)}{roster.length === 0 && <tr><td colSpan={5} className="px-5 py-12 text-center text-sm text-[var(--muted)]">ยังไม่มีรายชื่อจากเอกสาร</td></tr>}</tbody></table></div>
        </div>}

        {panel === "members" && <div className="flex-1">
          <div className="flex flex-wrap items-center gap-2.5 border-b border-[var(--line)] px-5 py-3.5 lg:px-6">
            <span className="inline-flex items-baseline gap-1.5 rounded-lg border border-[var(--line)] px-3 py-2 text-xs text-[var(--muted)]"><b className="text-base font-semibold text-[var(--navy)]">{linkedCount - linkedByPosition}</b>ผูกจากชื่อ</span>
            <span className="inline-flex items-baseline gap-1.5 rounded-lg border border-[var(--line)] px-3 py-2 text-xs text-[var(--muted)]"><b className="text-base font-semibold text-[var(--navy)]">{linkedByPosition}</b>ผูกจากตำแหน่ง</span>
            <button type="button" aria-pressed={onlyUnlinked} onClick={() => setOnlyUnlinked((value) => !value)} className={`focus-ring inline-flex items-baseline gap-1.5 rounded-lg border px-3 py-2 text-xs ${onlyUnlinked ? "border-[#f0882a] bg-[#f0882a] text-white" : "border-[#f0882a] bg-[#fff5ec] text-[#9a4a0c]"}`}><b className="text-base font-semibold">{unlinkedAppointments.length}</b>ยังไม่ผูก · {onlyUnlinked ? "แสดงทั้งหมด" : "ดูเฉพาะแถวนี้"}</button>
            {editable && (pendingGuesses.length > 0 || missingHolders > 0 || missingPeople > 0) && <span aria-hidden="true" className="mx-1 h-7 w-px bg-[var(--line)]" />}
            {editable && pendingGuesses.length > 0 && <button type="button" onClick={acceptAllGuesses} className="focus-ring inline-flex h-10 items-center gap-1.5 rounded-lg bg-[var(--accent)] px-3.5 text-[13px] font-medium text-white hover:bg-[var(--accent-strong)]"><Check size={15} weight="bold" />ใช้คนที่ระบบเดาทั้งหมด ({pendingGuesses.length})</button>}
            {editable && missingHolders > 0 && <Link href="/settings/teachers#vacant-positions" className="focus-ring inline-flex h-10 items-center rounded-lg border border-[var(--accent)] px-3.5 text-[13px] font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]">+ กรอกผู้ดำรงตำแหน่งที่ขาด ({missingHolders})</Link>}
            {editable && missingPeople > 0 && <Link href="/settings/teachers" className="focus-ring inline-flex h-10 items-center rounded-lg border border-[var(--accent)] px-3.5 text-[13px] font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]">+ เพิ่มคนที่ไม่พบในระบบ ({missingPeople})</Link>}
            <input value={memberQuery} onChange={(event) => setMemberQuery(event.target.value)} aria-label="ค้นหารายชื่อ" placeholder="ค้นหาชื่อหรือตำแหน่ง" className="focus-ring ml-auto h-10 w-full rounded-full border border-[var(--line-strong)] bg-white px-4 text-[13px] outline-none focus:border-[var(--accent)] sm:w-64" />
          </div>
          <div className="overflow-x-auto"><table className="data-grid w-full min-w-[860px] text-left text-sm"><thead><tr><th className="w-14 px-5 py-3 lg:px-6">#</th><th className="px-4 py-3">ชื่อ</th><th className="px-4 py-3">ฝ่าย</th><th className="px-4 py-3">ตำแหน่ง</th><th className="w-[32%] px-5 py-3 lg:px-6">บุคคลในระบบ</th></tr></thead>
            <tbody>{memberRows.map(({ row, no }) => {
              const state = matchState(row);
              const guess = guessFor(row);
              const showGuess = guess && !rejectedGuesses.has(row.id);
              const open = openRows[row.id] ?? false;
              return <Fragment key={row.id}>
                <tr className={selectedAppointmentId === row.id ? "bg-[var(--brand-blue-pale)]" : ""}>
                  <td className="px-5 py-3 text-[var(--subtle)] lg:px-6">{no}</td>
                  <td className="px-4 py-3">{row.fullName.trim() || "-"}</td>
                  <td className="px-4 py-3">{departmentOf(row)}</td>
                  <td className="px-4 py-3 text-[var(--muted)]">{[row.position, row.committeeRole].filter(Boolean).join(" · ") || "-"}</td>
                  <td className="px-5 py-2 lg:px-6">{isLinked(row)
                    ? <div className="flex flex-wrap items-center gap-x-2 gap-y-1"><span className="font-medium">{row.directoryUser?.fullName || row.fullName}</span>{linkedBy[row.nameMatchMethod ?? ""] && <span className="rounded-full bg-[var(--line-soft)] px-2 py-0.5 text-[11px] text-[var(--muted)]">{linkedBy[row.nameMatchMethod ?? ""]}</span>}{editable && <button type="button" onClick={() => toggleRow(row.id, open)} className="focus-ring rounded text-xs text-[var(--accent)] hover:underline">{open ? "ปิด" : "เปลี่ยน"}</button>}</div>
                    : showGuess
                      ? <select disabled={!editable} aria-label={`เลือกบุคคลสำหรับ ${personLabel(row)}`} value={String(guess.userId)} onChange={(event) => { if (event.target.value === "other") { rejectGuess(row.id); setOpenRows((rows) => ({ ...rows, [row.id]: true })); } else if (event.target.value === "none") rejectGuess(row.id); else { const chosen = row.candidates?.find((candidate) => String(candidate.userId) === event.target.value); if (chosen) linkAppointment(row.id, chosen.userId, chosen.fullName, chosen.positionTitle); } }} className="focus-ring h-9 w-full rounded-md border border-[#f0882a] bg-[#fffaf5] px-2.5 text-[13px]">
                        {row.candidates?.map((candidate) => <option key={candidate.userId} value={candidate.userId}>{candidate.fullName}{candidate.userId === guess.userId ? " (ระบบเดา)" : ""}</option>)}
                        <option value="none">ไม่ใช่คนนี้ · ปล่อยว่างไว้ก่อน</option>
                        <option value="other">ค้นหาหรือเพิ่มคนอื่น…</option>
                      </select>
                      : <button type="button" disabled={!editable} aria-expanded={open} onClick={() => toggleRow(row.id, open)} className="focus-ring flex h-9 w-full items-center justify-between rounded-md border border-dashed border-[#f0882a] bg-[#fffaf5] px-2.5 text-left text-[13px] text-[#9a4a0c]">{row.fullName.trim() ? state.label : "เลือกผู้ดำรงตำแหน่ง"} · เลือกบุคคล…<span aria-hidden="true">{open ? "▴" : "▾"}</span></button>}</td>
                </tr>
                {open && <tr><td colSpan={5} className="space-y-3 bg-[var(--surface-muted)] px-5 py-4 lg:px-6">
                  <fieldset disabled={!editable} className="grid gap-2 sm:grid-cols-3">
                    <label className="block"><span className="text-[11px] text-[var(--muted)]">ชื่อตามเอกสาร</span><input value={row.fullName} onChange={(event) => updateAppointment(row.id, "fullName", event.target.value)} className={`${inputClass} mt-1`} /></label>
                    <label className="block"><span className="text-[11px] text-[var(--muted)]">ตำแหน่ง</span><input value={row.position} onChange={(event) => updateAppointment(row.id, "position", event.target.value)} className={`${inputClass} mt-1`} /></label>
                    <label className="block"><span className="text-[11px] text-[var(--muted)]">บทบาท</span><input value={row.committeeRole} onChange={(event) => updateAppointment(row.id, "committeeRole", event.target.value)} className={`${inputClass} mt-1`} /></label>
                  </fieldset>
                  {isLinked(row)
                    ? <p className="flex flex-wrap items-center gap-2 text-xs text-[var(--muted)]"><Check size={14} className="text-[var(--success)]" />{row.directoryUser?.fullName ? `ผูกกับ ${row.directoryUser.fullName}` : "ผูกกับบุคคลในระบบแล้ว"} · {state.detail}<button disabled={!editable} type="button" onClick={() => unlinkAppointment(row.id)} className={`${smallButton} h-8 border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--line-soft)]`}><ArrowCounterClockwise size={13} /> เปลี่ยนคน</button></p>
                    : <fieldset disabled={!editable}><UnlinkedResolver row={row} state={state} picker={picker} draft={draft} onPick={linkAppointment} onCompare={() => openCompare(row)} onPickerChange={setPicker} onSearch={runDirectorySearch} onDraftChange={setDraft} onOpenDraft={() => openDraft(row)} onSubmitDraft={submitDraft} /></fieldset>}
                  <div className="flex gap-2"><button type="button" onClick={() => locateAppointment(row.id, row.pageNo)} className={`${smallButton} border-[var(--line-strong)] bg-white text-[var(--accent)] hover:bg-[var(--line-soft)]`}><MapPin size={14} /> ดูกรอบในเอกสาร</button><button disabled={!editable} type="button" onClick={() => removeAppointment(row.id)} className={`${smallButton} border-transparent text-[var(--muted)] hover:text-[var(--danger)]`}><Trash size={13} /> ลบแถว</button></div>
                </td></tr>}
              </Fragment>;
            })}{memberRows.length === 0 && <tr><td colSpan={5} className="p-10 text-center text-sm text-[var(--muted)]">{onlyUnlinked ? "ผูกครบทุกชื่อแล้ว" : "ไม่พบรายการตามที่ค้นหา"}</td></tr>}</tbody>
          </table></div>
        </div>}

        {panel === "metadata" && <div className="grid flex-1 content-start gap-4 p-5 sm:grid-cols-2 lg:grid-cols-3 lg:p-6">{metadataFields.map((field) => <label className={`block ${field.fieldKey === "committee_name" ? "sm:col-span-2" : ""}`} key={field.id}><span className="text-xs text-[var(--muted)]">{field.fieldLabel}</span><input disabled={!editable} value={field.value} onChange={(event) => updateField(field.id, event.target.value)} className={`focus-ring mt-1.5 h-10 w-full rounded-md border px-3 text-sm font-medium outline-none focus:border-[var(--accent)] ${field.confidence < reviewThreshold || !field.value ? "border-[var(--warning-dot)] bg-[#fffbf0]" : "border-[var(--line-strong)]"}`} /><span className="mt-1 block text-[11px] text-[var(--muted)]">หน้า {field.pageNo || "-"} · ความมั่นใจ {Math.round(field.confidence * 100)}% · {field.sourceText || "ไม่พบข้อความอ้างอิง"}</span>{(() => { const reading = dateReading(field, document.fields?.find((item) => item.id === field.id)); return reading && <span className={`mt-0.5 block text-[11px] ${reading.ok ? "text-[var(--success)]" : "font-medium text-[var(--danger)]"}`}>{reading.text}</span>; })()}</label>)}{metadataFields.length === 0 && <p className="text-sm text-[var(--muted)]">ยังไม่มีข้อมูลคำสั่งจาก OCR</p>}</div>}

        {panel === "duties" && <div className="flex flex-1 flex-col"><DepartmentDutiesPanel appointments={appointments} editable={editable} onChange={(update) => setAppointments(update)} /><details className="border-t border-[var(--line)] px-5 py-4 lg:px-6"><summary className="focus-ring min-h-10 cursor-pointer rounded py-2 text-sm font-medium text-[var(--navy)]">หน้าที่โดยรวมของคณะกรรมการ และการอ้างอิงประกาศเพิ่มเติม</summary><div className="mt-3 space-y-5"><label className="block"><span className="text-sm font-medium text-[var(--navy)]">หน้าที่รับผิดชอบของคณะกรรมการ</span><textarea readOnly={!editable} value={document.responsibilities} onChange={(event) => setDocument((current) => ({ ...current, responsibilities: event.target.value }))} rows={8} className="focus-ring mt-2 w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6 outline-none focus:border-[var(--accent)]" /><span className="mt-1 block text-xs text-[var(--muted)]">แก้ไขข้อความจากเอกสารได้ แล้วกดบันทึกฉบับร่าง</span></label><label className="block"><span className="text-sm font-medium text-[var(--navy)]">การอ้างอิงประกาศเพิ่มเติม</span><textarea readOnly={!editable} value={document.additionalReferences} onChange={(event) => setDocument((current) => ({ ...current, additionalReferences: event.target.value }))} rows={4} className="focus-ring mt-2 w-full rounded-md border border-[var(--line-strong)] px-3 py-2 text-sm leading-6 outline-none focus:border-[var(--accent)]" /></label></div></details></div>}
      </section>}
    {panel === "history" && <section className="card overflow-hidden"><div className="border-b border-[var(--line)] px-5 py-4 lg:px-6"><h2 className="text-base font-semibold text-[var(--navy)]">ประวัติการแก้ไข</h2><p className="mt-0.5 text-[13px] font-light text-[var(--muted)]">บันทึกการเปลี่ยนแปลงข้อมูลและการยกเลิกเอกสาร</p></div><div className="overflow-x-auto"><table className="data-grid w-full min-w-[620px] text-left text-sm"><thead><tr><th className="px-6 py-3">ฟิลด์</th><th className="px-4 py-3">ค่าเดิม</th><th className="px-4 py-3">ค่าที่แก้ไข</th><th className="px-6 py-3">ผู้แก้ไข / เวลา</th></tr></thead><tbody>{revisions.map((revision) => <tr key={revision.id}><td className="px-6 py-3 font-medium">{revision.fieldKey === "cancellation_reason" ? "เหตุผลที่ยกเลิก" : revision.fieldKey === "status" ? "สถานะเอกสาร" : revision.fieldKey}</td><td className="max-w-[180px] truncate px-4 py-3 text-[var(--muted)]">{revision.fieldKey === "status" ? statusLabel(revision.oldValue) : revision.oldValue || "-"}</td><td className="max-w-[180px] truncate px-4 py-3">{revision.fieldKey === "status" ? statusLabel(revision.newValue) : revision.newValue || "-"}</td><td className="px-6 py-3 text-xs text-[var(--muted)]">{revision.changedBy}<br />{formatThaiDate(revision.changedAt)}</td></tr>)}{revisions.length === 0 && <tr><td colSpan={4} className="px-6 py-12 text-center text-[var(--muted)]">ยังไม่มีประวัติการแก้ไขเอกสาร</td></tr>}</tbody></table></div></section>}
    </div>
        {editable && <div className="card flex flex-col gap-3 border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 sm:flex-row sm:items-center sm:justify-between lg:px-6">
          <div className="flex min-w-0 items-center gap-2 text-[13px] leading-5 sm:flex-1">{replaceArmed ? <><WarningCircle size={18} weight="fill" className="shrink-0 text-[var(--danger)]" /><span className="text-[var(--danger)]">ไฟล์ใหม่จะแทนที่ไฟล์เดิม ข้อมูลที่ตรวจไว้จะถูกสกัดใหม่ทั้งหมด</span><button type="button" onClick={() => setReplaceArmed(false)} className="focus-ring shrink-0 text-[var(--muted)] underline hover:text-[var(--ink)]">ยกเลิก</button></> : unlinkedAppointments.length > 0 ? <><Info size={18} className="shrink-0 text-[#9a4a0c]" /><span className="text-[#9a4a0c]">ยังไม่ผูก {unlinkedAppointments.length} ชื่อ · ยืนยันได้เลย ชื่อเหล่านี้เก็บตามเอกสารไว้ก่อน ผูกทีหลังได้</span></> : document.status === "CONFIRMED" ? <><CheckCircle size={18} weight="fill" className="shrink-0 text-[var(--success)]" /><span>ยืนยันข้อมูลแล้วโดย {document.confirmedBy || "เจ้าหน้าที่"}</span></> : <><Info size={18} className="shrink-0 text-[var(--accent)]" /><span className="text-[var(--muted)]">ระบบจะไม่ถือเป็นข้อมูลสมบูรณ์จนกว่าจะยืนยัน</span></>}</div>
          <div className="flex shrink-0 flex-wrap justify-end gap-2"><input ref={replaceInputRef} id="replace-document-file" type="file" accept=".pdf,.docx,.xlsx,.csv" className="sr-only" onChange={onReplaceFileChosen} /><button type="button" disabled={replacing || saving} onClick={() => { if (!replaceArmed) { setReplaceArmed(true); return; } replaceInputRef.current?.click(); }} className={`focus-ring inline-flex h-11 items-center gap-2 whitespace-nowrap rounded-md px-4 text-sm transition disabled:opacity-50 ${replaceArmed ? "bg-[var(--danger)] font-medium text-white" : "text-[var(--muted)] hover:bg-[var(--line-soft)] hover:text-[var(--ink)]"}`}><UploadSimple size={17} />{replacing ? "กำลังอัปโหลด..." : replaceArmed ? "เลือกไฟล์ที่จะทับ" : "อัปโหลดไฟล์ใหม่ทับ"}</button><button type="button" disabled={saving} onClick={() => saveDraft(false)} className="focus-ring inline-flex h-11 items-center whitespace-nowrap rounded-md border border-[var(--accent)] bg-white px-4 text-sm font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)] disabled:opacity-50">บันทึกฉบับร่าง</button><button type="button" disabled={saving || document.status === "CONFIRMED"} onClick={() => saveDraft(true)} className="focus-ring inline-flex h-11 items-center whitespace-nowrap rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{saving ? "กำลังบันทึก..." : "บันทึกและยืนยันข้อมูล"}</button></div>
        </div>}

    {editable && compare && <CandidateCompareDialog compare={compare} onSelect={(userId) => setCompare({ ...compare, selected: userId })} onClose={() => setCompare(null)} onConfirm={(candidate) => linkAppointment(compare.appointment.id, candidate.userId, candidate.fullName, candidate.positionTitle)} onCreate={() => { const row = compare.appointment; setCompare(null); openDraft(row); }} />}
  </div></AppShell>;
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
