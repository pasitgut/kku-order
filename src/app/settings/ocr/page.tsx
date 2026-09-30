"use client";

import { ChangeEvent, useEffect, useState } from "react";
import { FilePdf, FolderSimple, GoogleDriveLogo, Info } from "@phosphor-icons/react";
import { useMe } from "@/components/me-context";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { useToast } from "@/components/toast";
import { OCRSettings, OCRTestResult, getOCRSettings, getSystemSettings, testOCRSettings, updateOCRSettings } from "@/lib/api";
import { can } from "@/lib/permissions";

const engines = [
  { id: "one-ocr", name: "one-ocr", description: "ทำงานในเครื่อง รองรับไทย อังกฤษ และเลขไทย ข้อมูลไม่ออกนอกระบบ", status: "ติดตั้งแล้ว", available: true },
  { id: "tesseract", name: "Tesseract", description: "ทำงานในเครื่อง โอเพนซอร์ส ต้องติดตั้งชุดภาษาไทยเพิ่ม", status: "ยังไม่รองรับ", available: false },
  { id: "external", name: "OCR ผ่าน API ภายนอก", description: "ส่งภาพไปประมวลผลที่บริการภายนอก ต้องมี URL และ API key", status: "ยังไม่รองรับ", available: false },
];

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
  const [sample, setSample] = useState<File | null>(null);
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<OCRTestResult | null>(null);

  useEffect(() => {
    getOCRSettings().then((response) => { setSaved(response.data); setForm(response.data); }).catch((reason: Error) => setError(reason.message));
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

  const runTest = async () => {
    if (!form || !sample) return;
    setTesting(true);
    setResult(null);
    try {
      const response = await testOCRSettings(sample, form);
      setResult(response.data);
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "ทดสอบไม่สำเร็จ", { kind: "error" });
    } finally {
      setTesting(false);
    }
  };

  const onSample = (event: ChangeEvent<HTMLInputElement>) => { setSample(event.target.files?.[0] ?? null); setResult(null); };

  if (!form) return <SettingsFrame note="เปลี่ยนค่าในหน้านี้ได้โดยไม่ต้องแก้ .env">{error ? <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div> : <div className="card p-8"><div className="skeleton h-6 w-48" /><div className="skeleton mt-3 h-4 w-72" /></div>}</SettingsFrame>;

  const threshold = Math.round(form.reviewThreshold * 100);

  return <SettingsFrame note="เปลี่ยนค่าในหน้านี้ได้โดยไม่ต้องแก้ .env ค่าที่บันทึกจะใช้กับเอกสารที่นำเข้าหลังจากนี้">
    {!editable && <div className="flex items-center gap-3 rounded-lg border border-[var(--line)] bg-white px-4 py-3 text-[13px] text-[var(--muted)]"><Info size={18} className="shrink-0 text-[var(--accent)]" />ดูค่าได้อย่างเดียว เฉพาะผู้ดูแลระบบ (ADMIN) ที่แก้ไขและทดสอบได้</div>}

    <SettingsCard title="เครื่องมือ OCR" description="ตัวอ่านข้อความจากภาพเอกสาร">
      <div role="radiogroup" aria-label="เครื่องมือ OCR" className="grid gap-3.5 p-5 md:grid-cols-3 lg:px-6">
        {engines.map((engine) => {
          const active = form.engine === engine.id;
          return <button key={engine.id} type="button" role="radio" aria-checked={active} disabled={!engine.available || !editable} onClick={() => patch({ engine: engine.id })} className={`focus-ring flex flex-col gap-2 rounded-[10px] p-4 text-left transition disabled:cursor-not-allowed ${active ? "border-2 border-[var(--accent)] bg-[var(--brand-blue-pale)]" : "border border-[var(--line-strong)] bg-white enabled:hover:border-[var(--accent)]"} ${engine.available ? "" : "opacity-60"}`}>
            <span className="flex items-center justify-between gap-2"><span className="font-semibold text-[var(--navy)]">{engine.name}</span><span aria-hidden="true" className={`h-[18px] w-[18px] rounded-full ${active ? "border-[5px] border-[var(--accent)] bg-white" : "border-[1.5px] border-[#c2ccd6]"}`} /></span>
            <span className="text-xs font-light leading-5 text-[var(--muted)]">{engine.description}</span>
            <span className="flex items-center gap-1.5 text-xs text-[var(--muted)]"><StatusDot tone={engine.available ? "good" : "idle"} />{active ? "ใช้งานอยู่" : engine.status}</span>
          </button>;
        })}
      </div>
    </SettingsCard>

    <div className="grid items-start gap-6 xl:grid-cols-3">
      <div className="space-y-6 xl:col-span-2">
        <SettingsCard title="การสแกนและปรับภาพ" description="ค่าที่ใช้แปลง PDF เป็นภาพก่อนอ่านข้อความ">
          <Row title="ภาษาในเอกสาร" hint="อัตโนมัติจะตรวจภาษาเองทีละบรรทัด"><Segmented label="ภาษาในเอกสาร" value={form.language} disabled={!editable} options={[["auto", "อัตโนมัติ"], ["thai", "ไทย"], ["english", "อังกฤษ"]]} onChange={(language) => patch({ language })} /></Row>
          <Row title="ความละเอียด (DPI)" hint={form.dpi < 300 ? <span className="inline-flex items-center gap-1.5"><StatusDot tone="warn" />ต่ำกว่าที่ TOR แนะนำ (300 DPI)</span> : "ละเอียดขึ้นอ่านแม่นขึ้น แต่ใช้เวลานานขึ้น"}><Segmented label="ความละเอียด" value={form.dpi} disabled={!editable} options={[[150, "150"], [200, "200"], [300, "300"], [400, "400"]]} onChange={(dpi) => patch({ dpi })} /></Row>
          <Row title="ขนาดภาพสูงสุด" hint="ย่อด้านยาวของภาพก่อนอ่าน ลดเวลาและหน่วยความจำ" htmlFor="max-side"><select id="max-side" value={form.maxSide} disabled={!editable} onChange={(event) => patch({ maxSide: Number(event.target.value) })} className="focus-ring h-10 min-w-[160px] rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-sm disabled:opacity-60">{[1024, 1536, 2048, 3072].map((size) => <option key={size} value={size}>{size.toLocaleString("th-TH")} px</option>)}</select></Row>
          <Row title="ปรับภาพอัตโนมัติก่อนอ่าน" hint={<>แก้ภาพเอียงและเพิ่มความสว่าง ใช้เฉพาะเอกสารสแกนที่เอียง<br /><span className="inline-flex items-center gap-1.5"><StatusDot tone="warn" />กับ PDF ที่คมชัดอยู่แล้วอาจอ่านแย่ลง ทดสอบก่อนเปิดใช้</span></>}><span id="preprocess-label" className="sr-only">ปรับภาพอัตโนมัติก่อนอ่าน</span><Switch checked={form.preprocess} disabled={!editable} labelledBy="preprocess-label" onChange={(preprocess) => patch({ preprocess })} /></Row>
          <Row title="เกณฑ์ความมั่นใจ" hint="เอกสารหรือรายชื่อที่อ่านได้ต่ำกว่านี้ จะถูกทำเครื่องหมายให้ตรวจสอบ" htmlFor="threshold"><span className="flex items-center gap-3"><input id="threshold" type="range" min={50} max={100} step={5} value={threshold} disabled={!editable} onChange={(event) => patch({ reviewThreshold: Number(event.target.value) / 100 })} className="w-[200px] accent-[var(--accent)]" /><span className="w-12 text-right text-[15px] font-semibold text-[var(--navy)]">{threshold}%</span></span></Row>
        </SettingsCard>

        <SettingsCard title="ดึงเอกสารอัตโนมัติ" description="ระบบตรวจแหล่งไฟล์ตามรอบเวลา นำเข้า PDF ใหม่เข้าคิว OCR เอง และข้ามไฟล์ซ้ำ" action={<><span id="import-label" className="sr-only">เปิดการดึงเอกสารอัตโนมัติ</span><Switch checked={form.importEnabled} disabled={!editable} labelledBy="import-label" onChange={(importEnabled) => patch({ importEnabled })} /></>}>
          <div className={form.importEnabled ? "" : "opacity-50"}>
            <Row title="แหล่งไฟล์"><Segmented label="แหล่งไฟล์" value={form.importSource} disabled={!editable || !form.importEnabled} options={[["drive", "Google Drive"], ["folder", "โฟลเดอร์บนเซิร์ฟเวอร์"]]} onChange={(importSource) => patch({ importSource })} /></Row>
            {form.importSource === "drive" ? <>
              <Row title="บัญชี Google" hint="การเชื่อมต่อ Google Drive อยู่ระหว่างพัฒนา ตอนนี้บันทึกโฟลเดอร์ไว้ล่วงหน้าได้"><span className="flex shrink-0 flex-wrap items-center gap-3"><span className="inline-flex items-center gap-1.5 whitespace-nowrap text-[13px] text-[var(--muted)]"><StatusDot tone="idle" />ยังไม่ได้เชื่อมต่อ</span><button type="button" disabled className="inline-flex h-9 cursor-not-allowed items-center gap-2 whitespace-nowrap rounded-md border border-[var(--line-strong)] bg-white px-3 text-xs text-[var(--muted)]"><GoogleDriveLogo size={16} />เชื่อมต่อบัญชี Google</button></span></Row>
              <Row title="โฟลเดอร์ใน Google Drive" hint="วางลิงก์โฟลเดอร์จาก drive.google.com" htmlFor="drive-folder"><input id="drive-folder" type="url" value={form.driveFolderUrl} disabled={!editable || !form.importEnabled} onChange={(event) => patch({ driveFolderUrl: event.target.value.trim() })} placeholder="https://drive.google.com/drive/folders/..." className="focus-ring h-10 w-full rounded-md border border-[var(--line-strong)] px-3 text-[13px] disabled:opacity-60 sm:w-[340px]" /></Row>
              <Row title="หลังนำเข้าแล้ว" hint="ไฟล์ใน Drive ไม่ถูกลบ เพื่อให้ตรวจย้อนหลังได้" htmlFor="after-import"><select id="after-import" value={form.driveAfterImport} disabled={!editable || !form.importEnabled} onChange={(event) => patch({ driveAfterImport: event.target.value as OCRSettings["driveAfterImport"] })} className="focus-ring h-10 min-w-[240px] rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-sm disabled:opacity-60"><option value="move">ย้ายไปโฟลเดอร์ย่อย “นำเข้าแล้ว”</option><option value="keep">เก็บไว้ที่เดิม</option></select></Row>
            </> : <Row title="โฟลเดอร์บนเซิร์ฟเวอร์" hint="ตั้งที่ IMPORT_DIR ใน .env · เหมาะกับเครื่องสแกนที่บันทึกลงโฟลเดอร์แชร์"><span className="inline-flex items-center gap-2 font-mono text-[13px]"><FolderSimple size={16} className="text-[var(--muted)]" />{importDir || "-"}</span></Row>}
            <Row title={form.importSource === "drive" ? "ดึงไฟล์ทุก" : "ตรวจโฟลเดอร์ทุก"} htmlFor="interval"><select id="interval" value={form.importIntervalMinutes} disabled={!editable || !form.importEnabled} onChange={(event) => patch({ importIntervalMinutes: Number(event.target.value) })} className="focus-ring h-10 min-w-[160px] rounded-md border border-[var(--line-strong)] bg-white px-2.5 text-sm disabled:opacity-60">{[[1, "1 นาที"], [5, "5 นาที"], [15, "15 นาที"], [30, "30 นาที"], [60, "1 ชั่วโมง"], [1440, "วันละครั้ง"]].map(([minutes, label]) => <option key={minutes} value={minutes}>{label}</option>)}</select></Row>
          </div>
        </SettingsCard>
      </div>

      <SettingsCard title="ทดสอบการอ่าน" description="ลองกับเอกสาร 1 หน้า ด้วยค่าที่ตั้งไว้ในหน้านี้ (ยังไม่ต้องบันทึก)">
        <div className="space-y-3.5 p-5">
          <label className="focus-ring flex cursor-pointer items-center gap-3 rounded-lg border-[1.5px] border-dashed border-[var(--brand-blue-light)] p-3.5 hover:border-[var(--accent)] hover:bg-[var(--brand-blue-pale)]">
            <input type="file" accept=".pdf,.png,.jpg,.jpeg" className="sr-only" disabled={!editable} onChange={onSample} />
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-[#f1f3f6] text-[var(--muted)]"><FilePdf size={18} /></span>
            <span className="min-w-0"><span className="block truncate text-[13px] font-medium">{sample ? sample.name : "เลือกไฟล์ตัวอย่าง"}</span><span className="block text-xs text-[var(--accent)]">{sample ? "เปลี่ยนไฟล์" : "PDF, PNG หรือ JPG ไม่เกิน 20 MB"}</span></span>
          </label>
          <button type="button" disabled={!editable || !sample || testing} onClick={runTest} className="focus-ring h-11 w-full rounded-md bg-[var(--accent)] text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{testing ? `กำลังอ่านด้วย ${form.engine} · ${form.dpi} DPI...` : "ทดสอบด้วยค่าที่ตั้งไว้"}</button>
          {testing && <span className="block h-1 overflow-hidden rounded-full bg-[var(--line)]"><span className="block h-full w-1/2 animate-pulse rounded-full bg-[var(--accent)]" /></span>}
          {result && <>
            <div className="grid grid-cols-3 rounded-lg border border-[var(--line)]">{[["ความมั่นใจ", `${Math.round(result.confidence * 100)}%`], ["เวลา", `${result.seconds} วิ`], ["บรรทัด", result.lineCount.toLocaleString("th-TH")]].map(([label, value]) => <div key={label} className="border-r border-[var(--line-soft)] px-3 py-2.5 last:border-0"><p className="text-[11px] text-[var(--muted)]">{label}</p><p className="text-base font-semibold text-[var(--navy)]">{value}</p></div>)}</div>
            <div><p className="mb-1.5 text-xs text-[var(--muted)]">ข้อความที่อ่านได้</p><div className="shell-scrollbar max-h-[320px] overflow-y-auto rounded-lg bg-[var(--surface-muted)] px-3.5 py-3 text-[13px] leading-7">{result.lines.map((line, index) => <p key={index} className={line.confidence < form.reviewThreshold ? "bg-[var(--warning-soft)]" : ""}>{line.text}</p>)}</div>
              <p className="mt-1.5 inline-flex items-center gap-1.5 text-xs text-[var(--muted)]"><span className="h-2.5 w-2.5 rounded-sm bg-[var(--warning-soft)]" />ต่ำกว่าเกณฑ์ {threshold}%</p></div>
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
