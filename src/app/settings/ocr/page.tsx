"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { ArrowRight, FolderSimple, Info } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { useToast } from "@/components/toast";
import { OCRConnectionResult, OCRSettings, getOCRSettings, getSystemSettings, testOCRConnection, updateOCRSettings } from "@/lib/api";
import { can } from "@/lib/permissions";
import { isInsecureURL } from "@/lib/processing-status";

const engines = [
  { id: "thaidoc", name: "thaidoc-extract", description: "ส่งไฟล์ PDF ไปอ่านที่ thaidoc-extract ผ่าน API ต้องมี URL และ API key" },
  { id: "docflow-contract", name: "DocFlow contract", description: "ส่งไฟล์ไปอ่านที่ API ที่ตอบตามรูปแบบ DocFlow contract ต้องมี URL และ API key" },
];

const connectionTone: Record<OCRConnectionResult["status"], "good" | "warn" | "bad"> = { ok: "good", unauthorized: "warn", unreachable: "bad" };

function Segmented<T extends string | number>({ label, value, options, onChange, disabled }: { label: string; value: T; options: [T, string][]; onChange: (value: T) => void; disabled?: boolean }) {
  return <div role="radiogroup" aria-label={label} className="inline-flex gap-0.5 rounded-lg bg-[#f1f3f6] p-[3px]">
    {options.map(([option, text]) => <button key={String(option)} type="button" role="radio" aria-checked={value === option} disabled={disabled} onClick={() => onChange(option)} className={`focus-ring h-9 rounded-md px-4 text-[13px] transition disabled:cursor-not-allowed ${value === option ? "bg-white font-medium text-[var(--navy)] shadow-[0_1px_3px_rgba(0,0,0,0.12)]" : "text-[var(--muted)] hover:text-[var(--navy)]"}`}>{text}</button>)}
  </div>;
}

function Switch({ checked, onChange, labelledBy, disabled }: { checked: boolean; onChange: (checked: boolean) => void; labelledBy: string; disabled?: boolean }) {
  return <button type="button" role="switch" aria-checked={checked} aria-labelledby={labelledBy} disabled={disabled} onClick={() => onChange(!checked)} className={`focus-ring flex h-7 w-12 shrink-0 items-center rounded-full p-[3px] transition disabled:cursor-not-allowed disabled:opacity-60 ${checked ? "justify-end bg-[var(--accent)]" : "justify-start bg-[#c9d1d9]"}`}><span className="h-[22px] w-[22px] rounded-full bg-white" /></button>;
}

function Row({ title, hint, children, htmlFor }: { title: string; hint?: React.ReactNode; children: React.ReactNode; htmlFor?: string }) {
  return <div className="flex flex-col gap-3 border-b border-[var(--line-soft)] px-5 py-4 last:border-0 sm:flex-row sm:items-center sm:justify-between lg:px-6">
    <div><label htmlFor={htmlFor} id={htmlFor ? `${htmlFor}-label` : undefined} className="text-sm font-medium">{title}</label>{hint && <div className="mt-0.5 text-xs font-light text-[var(--muted)]">{hint}</div>}</div>
    {children}
  </div>;
}

export default function OCRSettingsPage() {
  const toast = useToast();
  const { me } = useMe();
  const [saved, setSaved] = useState<OCRSettings | null>(null);
  const [form, setForm] = useState<OCRSettings | null>(null);
  const [importDir, setImportDir] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [apiKeyConfigured, setApiKeyConfigured] = useState(false);
  const [testing, setTesting] = useState(false);
  const [connection, setConnection] = useState<OCRConnectionResult | null>(null);

  useEffect(() => {
    getOCRSettings().then((response) => { setSaved(response.data); setForm(response.data); setApiKeyConfigured(response.ocrApiKeyConfigured); }).catch((reason: Error) => setError(reason.message));
    getSystemSettings().then((response) => setImportDir(response.data.importDir)).catch(() => setImportDir(""));
  }, []);

  const editable = can(me?.role ?? "", "settings");
  const dirty = Boolean(form && saved && JSON.stringify(form) !== JSON.stringify(saved));
  const patch = (value: Partial<OCRSettings>) => setForm((current) => current ? { ...current, ...value } : current);

  const save = async () => {
    if (!form) return;
    setSaving(true);
    try {
      const response = await updateOCRSettings(form);
      setSaved(response.data);
      setForm(response.data);
      toast("บันทึกการตั้งค่า OCR แล้ว · ใช้กับเอกสารที่นำเข้าต่อจากนี้");
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "บันทึกการตั้งค่าไม่สำเร็จ", { kind: "error" });
    } finally {
      setSaving(false);
    }
  };

  const runConnectionTest = async () => {
    if (!form) return;
    setTesting(true);
    setConnection(null);
    try {
      const response = await testOCRConnection({ engine: form.engine, apiUrl: form.apiUrl });
      setConnection(response.data);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "ทดสอบการเชื่อมต่อไม่สำเร็จ", { kind: "error" });
    } finally {
      setTesting(false);
    }
  };

  if (!form) return <SettingsFrame note="เปลี่ยนค่าในหน้านี้ได้โดยไม่ต้องแก้ .env">{error ? <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div> : <div className="card p-8"><div className="skeleton h-6 w-48" /><div className="skeleton mt-3 h-4 w-72" /></div>}</SettingsFrame>;

  const threshold = Math.round(form.reviewThreshold * 100);

  return <SettingsFrame note="เปลี่ยนค่าในหน้านี้ได้โดยไม่ต้องแก้ .env ค่าที่บันทึกจะใช้กับเอกสารที่นำเข้าหลังจากนี้">
    {!editable && <div className="flex items-center gap-3 rounded-lg border border-[var(--line)] bg-white px-4 py-3 text-[13px] text-[var(--muted)]"><Info size={18} className="shrink-0 text-[var(--accent)]" />ดูค่าได้อย่างเดียว เฉพาะผู้ดูแลระบบ (ADMIN) ที่แก้ไขและทดสอบได้</div>}

    <SettingsCard title="เครื่องมือ OCR" description="ตัวอ่านข้อความจากภาพเอกสาร">
      <div role="radiogroup" aria-label="เครื่องมือ OCR" className="grid gap-3.5 p-5 md:grid-cols-3 lg:px-6">
        {engines.map((engine) => {
          const active = form.engine === engine.id;
          return <button key={engine.id} type="button" role="radio" aria-checked={active} disabled={!editable} onClick={() => patch({ engine: engine.id })} className={`focus-ring flex flex-col gap-2 rounded-[10px] p-4 text-left transition disabled:cursor-not-allowed ${active ? "border-2 border-[var(--accent)] bg-[var(--brand-blue-pale)]" : "border border-[var(--line-strong)] bg-white enabled:hover:border-[var(--accent)]"}`}>
            <span className="flex items-center justify-between gap-2"><span className="font-semibold text-[var(--navy)]">{engine.name}</span><span aria-hidden="true" className={`h-[18px] w-[18px] rounded-full ${active ? "border-[5px] border-[var(--accent)] bg-white" : "border-[1.5px] border-[#c2ccd6]"}`} /></span>
            <span className="text-xs font-light leading-5 text-[var(--muted)]">{engine.description}</span>
            <span className="flex items-center gap-1.5 text-xs text-[var(--muted)]"><StatusDot tone={active ? "good" : "idle"} />{active ? "ใช้งานอยู่" : "พร้อมใช้งาน"}</span>
          </button>;
        })}
      </div>
    </SettingsCard>

    <div className="space-y-6">
      <SettingsCard title="การเชื่อมต่อ OCR API" description="ที่อยู่ของบริการอ่านเอกสาร ใช้กับเอ็นจิน thaidoc-extract และ DocFlow contract">
        {isInsecureURL(form.apiUrl) && <div role="alert" className="mx-5 mt-5 flex items-center gap-2 rounded-lg bg-[var(--warning-soft)] px-4 py-3 text-[13px] text-[var(--warning)] lg:mx-6"><StatusDot tone="warn" />การเชื่อมต่อไม่ได้เข้ารหัส แนะนำให้ใช้ https://</div>}
        <Row title="API URL" hint="เช่น https://ocr.example.ac.th" htmlFor="api-url"><input id="api-url" type="url" value={form.apiUrl} disabled={!editable} onChange={(event) => patch({ apiUrl: event.target.value.trim() })} placeholder="https://..." className="focus-ring h-10 w-full rounded-md border border-[var(--line-strong)] px-3 text-[13px] disabled:opacity-60 sm:w-[340px]" /></Row>
        <Row title="API key" hint="ตั้งค่าที่ OCR_API_KEY ในไฟล์ .env ไม่แสดงในหน้านี้"><span className="inline-flex items-center gap-1.5 text-[13px] text-[var(--muted)]"><StatusDot tone={apiKeyConfigured ? "good" : "idle"} />{apiKeyConfigured ? "ตั้งค่าแล้ว" : "ยังไม่ได้ตั้งค่า"}</span></Row>
        <Row title="ทดสอบการเชื่อมต่อ" hint={connection ? <span className="inline-flex items-center gap-1.5"><StatusDot tone={connectionTone[connection.status]} />{connection.detail}</span> : "ตรวจว่าเข้าถึง API และ API key ใช้ได้หรือไม่"}><button type="button" disabled={!editable || testing} onClick={runConnectionTest} className="focus-ring h-10 rounded-md border border-[var(--line-strong)] bg-white px-4 text-sm hover:bg-[var(--line-soft)] disabled:cursor-not-allowed disabled:opacity-50">{testing ? "กำลังทดสอบ..." : "ทดสอบการเชื่อมต่อ"}</button></Row>
      </SettingsCard>

      <SettingsCard title="การตรวจสอบ" description="เอกสารหรือรายชื่อที่อ่านได้ต่ำกว่าเกณฑ์นี้ จะถูกทำเครื่องหมายให้ตรวจสอบ">
          <Row title="เกณฑ์ความมั่นใจ" hint="เอกสารหรือรายชื่อที่อ่านได้ต่ำกว่านี้ จะถูกทำเครื่องหมายให้ตรวจสอบ" htmlFor="threshold"><span className="flex items-center gap-3"><input id="threshold" type="range" min={50} max={100} step={5} value={threshold} disabled={!editable} onChange={(event) => patch({ reviewThreshold: Number(event.target.value) / 100 })} className="w-[200px] accent-[var(--accent)]" /><span className="w-12 text-right text-[15px] font-semibold text-[var(--navy)]">{threshold}%</span></span></Row>
      </SettingsCard>

        <SettingsCard title="ดึงเอกสารอัตโนมัติ" description="ระบบตรวจแหล่งไฟล์ตามรอบเวลา นำเข้า PDF ใหม่เข้าคิว OCR เอง และข้ามไฟล์ซ้ำ" action={<><span id="import-label" className="sr-only">เปิดการดึงเอกสารอัตโนมัติ</span><Switch checked={form.importEnabled} disabled={!editable} labelledBy="import-label" onChange={(importEnabled) => patch({ importEnabled })} /></>}>
          <div className={form.importEnabled ? "" : "opacity-50"}>
            <Row title="แหล่งไฟล์" hint="เลือกว่าจะดึงเอกสารจาก Google Drive หรือโฟลเดอร์บนเครื่อง"><Segmented label="แหล่งไฟล์" value={form.importSource} disabled={!editable || !form.importEnabled} options={[["drive", "Google Drive"], ["folder", "โฟลเดอร์บนเครื่อง"]]} onChange={(importSource) => patch({ importSource })} /></Row>
            {form.importSource === "drive" ? <>
              <Row title="โฟลเดอร์ Google Drive" hint={form.driveFolderUrl ? "โฟลเดอร์ถูกกำหนดแล้ว" : "ยังไม่ได้กำหนดโฟลเดอร์ Google Drive"}>
                <Link href="/settings/drive" className="focus-ring inline-flex h-10 items-center gap-2 rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm font-medium text-[var(--accent)] hover:bg-[var(--surface-muted)]">จัดการ Google Drive <ArrowRight size={16} /></Link>
              </Row>
              <Row title="รอบนำเข้า" hint="ดึงรายวันตามเขตเวลาที่เลือก เปลี่ยนเวลาและทดสอบสิทธิ์ได้ที่หน้า Google Drive">
                <span className="text-sm text-[var(--muted)]">ทุกวัน {form.driveSyncTime} · {form.driveTimezone}</span>
              </Row>
            </> : <>
              <Row title="โฟลเดอร์บนเครื่อง" hint="ตั้ง IMPORT_DIR ใน .env · เหมาะกับเครื่องสแกนที่บันทึกลงโฟลเดอร์แชร์"><span className="inline-flex items-center gap-2 font-mono text-[13px]"><FolderSimple size={16} className="text-[var(--muted)]" />{importDir || "-"}</span></Row>
              <Row title="ตรวจโฟลเดอร์ทุก" htmlFor="interval"><select id="interval" value={form.importIntervalMinutes} disabled={!editable || !form.importEnabled} onChange={(event) => patch({ importIntervalMinutes: Number(event.target.value) })} className="focus-ring h-10 min-w-[160px] rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-sm disabled:opacity-60">{[[1, "1 นาที"], [5, "5 นาที"], [15, "15 นาที"], [30, "30 นาที"], [60, "1 ชั่วโมง"], [1440, "วันละครั้ง"]].map(([minutes, label]) => <option key={minutes} value={minutes}>{label}</option>)}</select></Row>
              <Row title="ใช้ Google Drive แทน" hint="ตั้ง service account โฟลเดอร์ปลายทาง และรอบนำเข้าได้แยกต่างหาก"><Link href="/settings/drive" className="focus-ring inline-flex h-10 items-center gap-2 rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm font-medium text-[var(--accent)] hover:bg-[var(--surface-muted)]">ตั้งค่า Google Drive <ArrowRight size={16} /></Link></Row>
            </>}
          </div>
        </SettingsCard>
    </div>

    {editable && <div className="card flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-6">
      {dirty ? <span className="inline-flex items-center gap-2 text-[13px] font-medium"><StatusDot tone="warn" />มีการเปลี่ยนแปลงที่ยังไม่บันทึก · ใช้กับเอกสารที่นำเข้าหลังบันทึกเท่านั้น</span> : <span className="text-[13px] text-[var(--muted)]">เอกสารเดิมจะไม่ถูกอ่านใหม่ ถ้าต้องการใช้ค่าใหม่ ใช้ “อัปโหลดไฟล์ใหม่ทับ” ในหน้าเอกสาร</span>}
      <div className="flex gap-2"><button type="button" disabled={!dirty || saving} onClick={() => setForm(saved)} className="focus-ring h-11 rounded-md border border-[var(--line-strong)] bg-white px-4 text-sm hover:bg-[var(--line-soft)] disabled:opacity-50">ยกเลิกการแก้ไข</button><button type="button" disabled={!dirty || saving} onClick={save} className="focus-ring h-11 rounded-md bg-[var(--accent)] px-5 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{saving ? "กำลังบันทึก..." : "บันทึกการตั้งค่า"}</button></div>
    </div>}
  </SettingsFrame>;
}
