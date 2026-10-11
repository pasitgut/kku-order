"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowClockwise, CloudArrowDown, Info, ShieldCheck } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { useToast } from "@/components/toast";
import { DriveAutomationStatus, getDriveAutomationStatus, getOCRSettings, getSystemSettings, OCRSettings, syncGoogleDriveNow, testGoogleDriveConnection, updateOCRSettings } from "@/lib/api";
import { can } from "@/lib/permissions";

const runLabels: Record<string, string> = { RUNNING: "กำลังสำรวจและนำเข้า", COMPLETED: "ตรวจครบแล้ว", PARTIAL: "เสร็จบางส่วน · มีรายการต้องตรวจสอบ" };

function formatDriveTime(value: string | null | undefined, timezone: string) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium", timeStyle: "short", timeZone: timezone }).format(new Date(value));
}

function Switch({ checked, onChange, labelledBy, disabled }: { checked: boolean; onChange: (checked: boolean) => void; labelledBy: string; disabled?: boolean }) {
  return <button type="button" role="switch" aria-checked={checked} aria-labelledby={labelledBy} disabled={disabled} onClick={() => onChange(!checked)} className={`focus-ring flex h-7 w-12 shrink-0 items-center rounded-full p-[3px] transition disabled:cursor-not-allowed disabled:opacity-60 ${checked ? "justify-end bg-[var(--accent)]" : "justify-start bg-[#c9d1d9]"}`}><span className="h-[22px] w-[22px] rounded-full bg-white" /></button>;
}

function SettingRow({ title, hint, children, htmlFor }: { title: string; hint?: React.ReactNode; children: React.ReactNode; htmlFor?: string }) {
  return <div className="flex flex-col gap-3 border-b border-[var(--line-soft)] px-5 py-4 last:border-0 sm:flex-row sm:items-center sm:justify-between lg:px-6">
    <div className="min-w-0"><label htmlFor={htmlFor} id={htmlFor ? `${htmlFor}-label` : undefined} className="text-sm font-medium">{title}</label>{hint && <p className="mt-0.5 max-w-[58ch] text-xs font-light leading-5 text-[var(--muted)]">{hint}</p>}</div>
    {children}
  </div>;
}

export default function GoogleDriveSettingsPage() {
  const toast = useToast();
  const { me } = useMe();
  const [saved, setSaved] = useState<OCRSettings | null>(null);
  const [form, setForm] = useState<OCRSettings | null>(null);
  const [credentialsConfigured, setCredentialsConfigured] = useState<boolean | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [testedFolder, setTestedFolder] = useState("");
  const [automation, setAutomation] = useState<DriveAutomationStatus | null>(null);
  const [statusError, setStatusError] = useState("");

  useEffect(() => {
    let active = true;
    getOCRSettings().then((response) => {
      if (!active) return;
      setSaved(response.data);
      setForm(response.data);
      setError("");
    }).catch((reason: Error) => {
      if (active) setError(reason.message);
    }).finally(() => {
      if (active) setLoading(false);
    });
    getSystemSettings().then((response) => {
      if (active) setCredentialsConfigured(response.data.googleDriveCredentialsConfigured);
    }).catch(() => {
      if (active) setCredentialsConfigured(null);
    });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const response = await getDriveAutomationStatus();
        if (active) { setAutomation(response.data); setStatusError(""); }
      } catch (reason) {
        if (active) setStatusError(reason instanceof Error ? reason.message : "อ่านความคืบหน้าไม่สำเร็จ");
      } finally {
        if (active) timer = setTimeout(refresh, 5000);
      }
    };
    void refresh();
    return () => { active = false; clearTimeout(timer); };
  }, []);

  const editable = can(me?.role ?? "", "settings");
  const dirty = Boolean(form && saved && JSON.stringify(form) !== JSON.stringify(saved));
  const patch = (value: Partial<OCRSettings>) => {
    setTestedFolder("");
    setForm((current) => current ? { ...current, ...value } : current);
  };

  const save = async () => {
    if (!form) return;
    if (form.importEnabled && form.importSource === "drive" && !form.driveFolderUrl.trim()) {
      toast("กรุณาใส่ลิงก์โฟลเดอร์ Google Drive ก่อนเปิดนำเข้าอัตโนมัติ", { kind: "error" });
      return;
    }
    setSaving(true);
    try {
      const response = await updateOCRSettings({ ...form, driveFolderUrl: form.driveFolderUrl.trim() });
      setSaved(response.data);
      setForm(response.data);
      toast("บันทึกการตั้งค่า Google Drive แล้ว");
      const status = await getDriveAutomationStatus().catch(() => null);
      if (status) setAutomation(status.data);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "บันทึกการตั้งค่าไม่สำเร็จ", { kind: "error" });
    } finally {
      setSaving(false);
    }
  };

  const testConnection = async () => {
    if (!form?.driveFolderUrl.trim()) {
      toast("กรุณาใส่ลิงก์โฟลเดอร์ Google Drive ก่อนทดสอบ", { kind: "error" });
      return;
    }
    setTesting(true);
    setTestedFolder("");
    try {
      const response = await testGoogleDriveConnection(form.driveFolderUrl.trim());
      setTestedFolder(response.data.folderName);
      toast(`เข้าถึงโฟลเดอร์ “${response.data.folderName}” ได้แล้ว`);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "ทดสอบการเข้าถึงไม่สำเร็จ", { kind: "error" });
    } finally {
      setTesting(false);
    }
  };

  const syncNow = async () => {
    setSyncing(true);
    try {
      const response = await syncGoogleDriveNow();
      toast(`เริ่มงานดึงไฟล์ #${response.data.runId} แล้ว ติดตามความคืบหน้าด้านล่างได้`);
      const status = await getDriveAutomationStatus().catch(() => null);
      if (status) setAutomation(status.data);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "ดึงไฟล์จาก Google Drive ไม่สำเร็จ", { kind: "error" });
    } finally {
      setSyncing(false);
    }
  };

  if (loading) return <SettingsFrame note="ใช้ service account ของระบบในการอ่านไฟล์จากโฟลเดอร์ที่กำหนด"><div className="card space-y-3 p-6"><div className="skeleton h-5 w-48" /><div className="skeleton h-4 w-72" /><div className="skeleton mt-5 h-12 w-full" /></div></SettingsFrame>;

  if (!form || !saved) return <SettingsFrame note="ใช้ service account ของระบบในการอ่านไฟล์จากโฟลเดอร์ที่กำหนด"><div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error || "โหลดการตั้งค่า Google Drive ไม่สำเร็จ"}</div></SettingsFrame>;

  const driveSelected = form.importSource === "drive";

  return <SettingsFrame note="credential จะอยู่บน backend และไฟล์ต้นฉบับใน Google Drive จะไม่ถูกย้ายหรือลบ">
    <div className="flex flex-col gap-4 border-b border-[var(--line)] pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <p className="text-xs font-semibold uppercase tracking-[0.12em] text-[var(--accent)]">แหล่งนำเข้าเอกสาร</p>
        <h2 className="mt-1 text-2xl font-semibold tracking-tight text-[var(--navy)]">Google Drive</h2>
        <p className="mt-1 max-w-[60ch] text-sm leading-6 text-[var(--muted)]">ดึงเอกสารทุกวันจากโฟลเดอร์และโฟลเดอร์ย่อย พร้อมนำเข้าเฉพาะไฟล์ใหม่</p>
      </div>
      <span className={`inline-flex w-fit items-center gap-2 rounded-full px-3 py-2 text-xs font-medium ${testedFolder ? "bg-[var(--success-soft)] text-[var(--success)]" : credentialsConfigured ? "bg-[var(--accent-soft)] text-[var(--navy-soft)]" : "bg-[var(--warning-soft)] text-[var(--warning)]"}`}>
        <StatusDot tone={testedFolder ? "good" : credentialsConfigured ? "info" : "warn"} />
        {testedFolder ? `เข้าถึงได้ · ${testedFolder}` : credentialsConfigured ? "มี credential บน backend" : "ต้องตั้งค่า service account"}
      </span>
    </div>

    {!editable && <div className="flex items-center gap-3 rounded-lg border border-[var(--line)] bg-white px-4 py-3 text-[13px] text-[var(--muted)]"><Info size={18} className="shrink-0 text-[var(--accent)]" />ดูค่าได้อย่างเดียว เฉพาะผู้ดูแลระบบ (ADMIN) ที่แก้ไข ทดสอบ และสั่งนำเข้าได้</div>}

    <SettingsCard title="การเข้าถึง Google Drive" description="ระบบใช้ service account ที่ตั้งค่าไว้บน backend ไม่เชื่อมบัญชี Google ส่วนตัวของผู้ใช้">
      <div className="grid gap-5 p-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center lg:px-6">
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--brand-blue-pale)] text-[var(--accent)]"><ShieldCheck size={21} weight="regular" /></span>
          <div className="min-w-0">
            <p className="text-sm font-medium text-[var(--navy)]">ข้อมูลรับรองของ service account</p>
            <p className="mt-1 text-[13px] leading-5 text-[var(--muted)]">{credentialsConfigured === null ? "ตรวจสถานะ backend ไม่ได้" : credentialsConfigured ? "พบไฟล์ credential บน backend แล้ว แชร์โฟลเดอร์ให้ service account มีสิทธิ์ Viewer แล้วทดสอบการเข้าถึง" : "ยังไม่พบไฟล์ credential ให้ผู้ดูแล deployment ตั้ง GOOGLE_DRIVE_CREDENTIALS_FILE และรีสตาร์ต backend"}</p>
            {automation?.serviceAccountEmail && <div className="mt-3 flex flex-wrap items-center gap-2"><span className="break-all rounded-md bg-[var(--surface-muted)] px-3 py-2 text-sm text-[var(--navy)]">{automation.serviceAccountEmail}</span><button type="button" className="focus-ring rounded-md px-3 py-2 text-xs font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]" onClick={async () => { try { await navigator.clipboard.writeText(automation.serviceAccountEmail); toast("คัดลอกอีเมลแล้ว"); } catch { toast("คัดลอกไม่ได้ กรุณาเลือกและคัดลอกอีเมล", { kind: "error" }); } }}>คัดลอกอีเมล</button></div>}
          </div>
        </div>
        <div className="flex flex-wrap gap-2 lg:justify-end">
          <button type="button" disabled={!editable || testing || !credentialsConfigured || !form.driveFolderUrl.trim()} onClick={testConnection} className="focus-ring inline-flex h-11 items-center justify-center gap-2 rounded-md border border-[var(--line-strong)] bg-white px-4 text-sm font-medium text-[var(--navy)] hover:bg-[var(--surface-muted)] disabled:cursor-not-allowed disabled:opacity-50">
            <ArrowClockwise size={17} className={testing ? "animate-spin" : ""} />{testing ? "กำลังทดสอบ..." : "ทดสอบการเข้าถึง"}
          </button>
          <button type="button" disabled={!editable || syncing || dirty || (automation?.run?.status === "RUNNING" && !automation.run.retryAt && (saved.importEnabled || automation.run.triggerType === "MANUAL")) || saved.importSource !== "drive" || !credentialsConfigured} onClick={syncNow} className="focus-ring inline-flex h-11 items-center justify-center gap-2 rounded-md bg-[var(--accent)] px-4 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">
            <CloudArrowDown size={17} className={syncing ? "animate-pulse" : ""} />{syncing ? "กำลังตรวจไฟล์..." : "ดึงไฟล์ตอนนี้"}
          </button>
        </div>
      </div>
      {credentialsConfigured === false && <div role="note" className="flex gap-2 border-t border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 text-xs leading-5 text-[var(--muted)] lg:px-6"><Info size={16} className="mt-0.5 shrink-0 text-[var(--accent)]" /><p>อย่าอัปโหลดไฟล์ JSON นี้ผ่านหน้าเว็บ ให้เก็บไว้ใน secrets ของ backend เท่านั้น แล้วแชร์โฟลเดอร์เป้าหมายให้ service account</p></div>}
    </SettingsCard>

    <SettingsCard title="โฟลเดอร์และการนำเข้า" description="สำรวจทุกโฟลเดอร์ย่อยและทางลัดที่บัญชีระบบเข้าถึงได้ ตรวจซ้ำด้วยรหัสไฟล์และเนื้อหา">
      <SettingRow title="ใช้ Google Drive เป็นแหล่งนำเข้า" hint="เมื่อปิด ระบบจะใช้โฟลเดอร์บนเซิร์ฟเวอร์ที่ตั้งไว้ในหน้า OCR แทน">
        <span id="drive-source-label" className="sr-only">ใช้ Google Drive เป็นแหล่งนำเข้า</span>
        <Switch checked={driveSelected} disabled={!editable} labelledBy="drive-source-label" onChange={(enabled) => patch({ importSource: enabled ? "drive" : "folder" })} />
      </SettingRow>
      <SettingRow title="นำเข้าอัตโนมัติ" hint="เริ่มวันละครั้งตามเวลาที่ตั้งไว้ งานที่ยังไม่จบจะทำต่อในนาทีถัดไป">
        <span id="drive-import-enabled-label" className="sr-only">นำเข้าไฟล์จากแหล่งที่เลือกโดยอัตโนมัติ</span>
        <Switch checked={form.importEnabled} disabled={!editable} labelledBy="drive-import-enabled-label" onChange={(importEnabled) => patch({ importEnabled })} />
      </SettingRow>
      <div className={driveSelected ? "" : "opacity-55"}>
        <SettingRow title="ลิงก์โฟลเดอร์ Google Drive" hint="วางลิงก์โฟลเดอร์ที่แชร์ให้ service account แล้ว" htmlFor="drive-folder-url">
          <input id="drive-folder-url" type="url" inputMode="url" autoComplete="url" value={form.driveFolderUrl} disabled={!editable} onChange={(event) => patch({ driveFolderUrl: event.target.value })} placeholder="https://drive.google.com/drive/folders/..." className="focus-ring h-11 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm disabled:opacity-60 sm:w-[min(100%,420px)]" />
        </SettingRow>
        {driveSelected && <>
          <SettingRow title="เวลาดึงไฟล์ทุกวัน" hint="ค่าเริ่มต้น 05:00 น. เปลี่ยนเวลาได้โดยไม่ต้องรีสตาร์ต" htmlFor="drive-sync-time">
            <input id="drive-sync-time" type="time" required value={form.driveSyncTime} disabled={!editable} onChange={(event) => patch({ driveSyncTime: event.target.value })} className="focus-ring h-11 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm disabled:opacity-60 sm:w-[180px]" />
          </SettingRow>
          <SettingRow title="เขตเวลา" hint="เวลานำเข้าจะอ้างอิงเขตเวลานี้" htmlFor="drive-timezone">
            <input id="drive-timezone" list="drive-timezones" value={form.driveTimezone} disabled={!editable} onChange={(event) => patch({ driveTimezone: event.target.value })} className="focus-ring h-11 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm disabled:opacity-60 sm:w-[240px]" />
            <datalist id="drive-timezones"><option value="Asia/Bangkok" /><option value="UTC" /><option value="Asia/Singapore" /></datalist>
          </SettingRow>
        </>}
      </div>
      <div className="flex items-start gap-2 border-t border-[var(--line)] bg-[var(--surface-muted)] px-5 py-3.5 text-xs leading-5 text-[var(--muted)] lg:px-6">
        <Info size={16} className="mt-0.5 shrink-0 text-[var(--accent)]" />
        <p>รองรับ PDF, DOCX, XLSX, CSV และการส่งออก Google Docs, Sheets, Slides, Drawings ไฟล์ไม่เกิน 50 MB (Google Workspace export ไม่เกิน 10 MB) ไฟล์ที่เคยนำเข้าสำเร็จจะไม่ดึงซ้ำแม้เปลี่ยนชื่อ ย้ายโฟลเดอร์ หรือแก้ไขเนื้อหา เปลี่ยนลิงก์ต้นทางได้โดยเก็บประวัติเดิมไว้ {" "}<Link href="/settings/ocr" className="font-medium text-[var(--accent)] underline decoration-[var(--brand-blue-light)] underline-offset-2 hover:text-[var(--accent-strong)]">ตั้งค่า OCR</Link></p>
      </div>
    </SettingsCard>

    <SettingsCard title="ความคืบหน้าการดึงไฟล์" description="สถานะอัปเดตทุก 5 วินาที งานที่ขาดช่วงจะเริ่มต่อจากจุดที่บันทึกไว้">
      <div className="space-y-5 p-5 lg:px-6">
        {statusError && <p role="alert" className="text-sm text-[var(--danger)]">{statusError}</p>}
        <div className="grid gap-4 text-sm sm:grid-cols-2">
          <div><p className="text-xs text-[var(--muted)]">รอบถัดไป / ทำงานต่อ</p><p className="mt-1 font-medium">{saved.importSource === "drive" && automation?.nextSyncAt ? formatDriveTime(automation?.nextSyncAt, saved.driveTimezone) : "ปิดการนำเข้าอัตโนมัติ"}</p><p className="mt-1 text-xs text-[var(--muted)]">{saved.driveTimezone}</p></div>
          <div><p className="text-xs text-[var(--muted)]">งานล่าสุด{automation?.run ? ` #${automation.run.id}` : ""}</p><p className="mt-1 font-medium">{automation?.run ? runLabels[automation.run.status] : "ยังไม่มีประวัติการดึงไฟล์"}</p>{automation?.run && <p className="mt-1 text-xs text-[var(--muted)]">เริ่ม {formatDriveTime(automation.run.startedAt, saved.driveTimezone)}{automation.run.finishedAt ? ` · จบ ${formatDriveTime(automation.run.finishedAt, saved.driveTimezone)}` : ""}</p>}</div>
        </div>
        {automation?.run ? <>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-4 border-y border-[var(--line-soft)] py-4 sm:grid-cols-3">
            {([
              ["นำเข้าใหม่", automation.counts.IMPORTED ?? 0],
              ["เคยนำเข้า / เนื้อหาซ้ำ", (automation.counts.EXISTING ?? 0) + (automation.counts.DUPLICATE ?? 0)],
              ["ไฟล์รอดึง", automation.counts.PENDING ?? 0],
              ["โฟลเดอร์ตรวจครบ", automation.folderCounts.COMPLETED ?? 0],
              ["ข้ามตามข้อจำกัด", automation.counts.SKIPPED ?? 0],
              ["ไม่สำเร็จ", (automation.counts.FAILED ?? 0) + (automation.folderCounts.FAILED ?? 0)],
            ] as const).map(([label, count]) => <div key={label}><dt className="text-xs text-[var(--muted)]">{label}</dt><dd className="mt-1 text-2xl font-semibold tabular-nums text-[var(--navy)]">{count.toLocaleString("th-TH")}</dd></div>)}
          </dl>
          {automation.run.status === "RUNNING" && <p className="text-xs leading-5 text-[var(--muted)]">{(saved.importEnabled || automation.run.triggerType === "MANUAL") ? "ระบบกำลังสำรวจไฟล์ จำนวนรอดึงอาจเพิ่มขึ้นเมื่อพบโฟลเดอร์ย่อยใหม่" : "การทำงานอัตโนมัติพักไว้ เปิดอีกครั้งหรือกดดึงไฟล์ตอนนี้เพื่อทำต่อ"} · โฟลเดอร์รอตรวจ {(automation.folderCounts.PENDING ?? 0).toLocaleString("th-TH")}</p>}
          {automation.run.lastError && <p role="status" className="rounded-md bg-[var(--warning-soft)] p-3 text-xs leading-5 text-[var(--warning)]">{automation.run.lastError}</p>}
          {automation.issues.length > 0 && <details className="border-t border-[var(--line-soft)] pt-4"><summary className="focus-ring cursor-pointer text-sm font-medium text-[var(--navy)]">ไฟล์และโฟลเดอร์ที่ต้องตรวจสอบ (แสดงไม่เกิน 40 รายการ)</summary><ul className="mt-3 divide-y divide-[var(--line-soft)]">{automation.issues.map((issue, index) => <li key={`${issue.fileName}-${index}`} className="py-3"><p className="break-all text-sm font-medium">{issue.fileName}</p><p className="mt-1 break-words text-xs leading-5 text-[var(--muted)]">{issue.lastError}</p></li>)}</ul></details>}
          {automation.history.length > 1 && <details className="border-t border-[var(--line-soft)] pt-4"><summary className="focus-ring cursor-pointer text-sm font-medium text-[var(--navy)]">ประวัติ 10 รอบล่าสุดของโฟลเดอร์นี้</summary><ul className="mt-3 divide-y divide-[var(--line-soft)]">{automation.history.map((run) => <li key={run.id} className="flex flex-wrap justify-between gap-2 py-3 text-xs"><span>#{run.id} · {formatDriveTime(run.startedAt, saved.driveTimezone)}</span><span>{runLabels[run.status]}</span></li>)}</ul></details>}
        </> : <p className="border-t border-[var(--line-soft)] pt-4 text-sm leading-6 text-[var(--muted)]">แชร์โฟลเดอร์ให้อีเมลระบบ ทดสอบการเข้าถึง แล้วบันทึกการตั้งค่า กด “ดึงไฟล์ตอนนี้” เพื่อเริ่มนำเข้าไฟล์เดิมทั้งหมดได้ทันที</p>}
      </div>
    </SettingsCard>

    <div className="card flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
      {dirty ? <span className="inline-flex items-center gap-2 text-[13px] font-medium"><StatusDot tone="warn" />มีการเปลี่ยนแปลงที่ยังไม่บันทึก</span> : <span className="text-[13px] text-[var(--muted)]">{saved.importSource === "drive" ? "Google Drive เป็นแหล่งนำเข้าปัจจุบัน" : "Google Drive ยังไม่ได้เลือกเป็นแหล่งนำเข้าปัจจุบัน"}</span>}
      <div className="flex flex-col-reverse gap-2 sm:flex-row">
        <button type="button" disabled={!dirty || saving} onClick={() => { setForm(saved); setTestedFolder(""); }} className="focus-ring h-11 rounded-md border border-[var(--line-strong)] bg-white px-4 text-sm hover:bg-[var(--surface-muted)] disabled:cursor-not-allowed disabled:opacity-50">ยกเลิกการแก้ไข</button>
        <button type="button" disabled={!editable || !dirty || saving} onClick={save} className="focus-ring h-11 rounded-md bg-[var(--accent)] px-5 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{saving ? "กำลังบันทึก..." : "บันทึกการตั้งค่า"}</button>
      </div>
    </div>
  </SettingsFrame>;
}
