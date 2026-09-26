"use client";

import { useEffect, useState } from "react";
import { Info } from "@phosphor-icons/react";
import { SettingsCard, SettingsFrame, StatusDot } from "@/components/settings-frame";
import { SyncRun, SystemSettings, formatThaiDate, getSyncRuns, getSystemSettings } from "@/lib/api";
import { cronLabel } from "@/lib/cron-label";

function EnvValue({ label, value, envKey, tone }: { label: string; value: string; envKey: string; tone?: "good" | "idle" }) {
  return <div className="flex items-start justify-between gap-4 border-b border-[var(--line-soft)] px-5 py-3.5 last:border-0 lg:px-6">
    <div><p className="text-sm">{label}</p><code className="text-[11px] text-[var(--subtle)]">{envKey}</code></div>
    <p className="flex items-center gap-2 text-right text-sm font-medium">{tone && <StatusDot tone={tone} />}{value}</p>
  </div>;
}

export default function SystemSettingsPage() {
  const [settings, setSettings] = useState<SystemSettings | null>(null);
  const [runs, setRuns] = useState<SyncRun[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([getSystemSettings(), getSyncRuns()]).then(([settingsResponse, runResponse]) => { setSettings(settingsResponse.data); setRuns(runResponse.data.slice(0, 5)); }).catch((reason: Error) => setError(reason.message));
  }, []);

  return <SettingsFrame note="ค่าในหน้านี้อ่านจากไฟล์ .env ของเซิร์ฟเวอร์ ส่วน OCR และการนำเข้าแก้ไขได้ในหน้าของตัวเอง">
    <div className="flex items-center gap-3 rounded-lg border border-[var(--line)] bg-white px-4 py-3 text-[13px] text-[var(--muted)]"><Info size={18} className="shrink-0 text-[var(--accent)]" />ค่าในหน้านี้แสดงอย่างเดียว เปลี่ยนได้ที่ไฟล์ .env แล้วรีสตาร์ต API · ชื่อตัวแปรอยู่ใต้แต่ละค่า</div>
    {error && <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}

    <SettingsCard id="sync" title="การ sync บุคลากร" description="ดึงรายชื่อจาก External API มาใช้จับคู่ชื่อในคำสั่ง">
      <div>
        <EnvValue label="รอบการ sync" value={settings ? cronLabel(settings.userSyncCron) : "-"} envKey={`USER_SYNC_CRON = ${settings?.userSyncCron ?? ""}`} />
        <EnvValue label="เขตเวลา" value={settings?.userSyncTimezone ?? "-"} envKey="USER_SYNC_TZ" />
        <EnvValue label="แหล่งข้อมูล" value={settings?.externalUsersHost || "ยังไม่ได้ตั้งค่า"} envKey="EXTERNAL_USERS_URL" tone={settings?.externalUsersHost ? "good" : "idle"} />
        <EnvValue label="API key" value={settings?.apiKeyConfigured ? "ตั้งค่าแล้ว" : "ยังไม่ได้ตั้งค่า"} envKey="API_KEY" tone={settings?.apiKeyConfigured ? "good" : "idle"} />
      </div>
      <div className="overflow-x-auto border-t border-[var(--line)]"><table className="data-grid w-full min-w-[560px] text-left text-sm"><thead><tr><th className="px-6 py-2.5">รอบ sync</th><th className="px-3 py-2.5">สถานะ</th><th className="px-3 py-2.5 text-right">ดึงมา</th><th className="px-3 py-2.5 text-right">เพิ่ม</th><th className="px-3 py-2.5 text-right">แก้ไข</th><th className="px-6 py-2.5 text-right">ไม่เปลี่ยน</th></tr></thead>
        <tbody>{runs.map((run) => <tr key={run.id}><td className="px-6 py-3">{formatThaiDate(run.startedAt)}</td><td className="px-3 py-3"><span className="inline-flex items-center gap-1.5 text-[13px] text-[var(--muted)]" title={run.errorMessage}><StatusDot tone={run.status === "SUCCESS" ? "good" : run.status === "RUNNING" ? "info" : "bad"} />{run.status === "SUCCESS" ? "สำเร็จ" : run.status === "RUNNING" ? "กำลังทำงาน" : "ล้มเหลว"}</span></td><td className="px-3 py-3 text-right">{run.fetched}</td><td className="px-3 py-3 text-right">{run.created}</td><td className="px-3 py-3 text-right">{run.updated}</td><td className="px-6 py-3 text-right text-[var(--muted)]">{run.unchanged}</td></tr>)}{runs.length === 0 && <tr><td colSpan={6} className="px-6 py-8 text-center text-sm text-[var(--muted)]">ยังไม่มีประวัติการ sync</td></tr>}</tbody></table></div>
    </SettingsCard>

    <SettingsCard id="expiry" title="การแจ้งเตือนหมดวาระ" description="ส่งอีเมลถึงผู้ถูกแต่งตั้งก่อนคำสั่งหมดวาระ">
      <EnvValue label="เกณฑ์แจ้งเตือน" value="3, 2 และ 1 เดือน" envKey="กำหนดในระบบ" />
      <EnvValue label="รอบตรวจวันหมดวาระ" value={settings ? cronLabel(settings.expiryCron) : "-"} envKey={`EXPIRY_CRON = ${settings?.expiryCron ?? ""}`} />
      <EnvValue label="เซิร์ฟเวอร์อีเมล" value={settings?.smtpConfigured ? `${settings.smtpHost} : ${settings.smtpPort}` : "ยังไม่ได้ตั้งค่า"} envKey="SMTP_HOST · SMTP_PORT" tone={settings?.smtpConfigured ? "good" : "idle"} />
      <EnvValue label="ผู้ส่ง" value={settings?.smtpFrom || "-"} envKey="SMTP_FROM" />
    </SettingsCard>
  </SettingsFrame>;
}
