"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Camera, Check, SignOut } from "@phosphor-icons/react";
import { AppShell } from "@/components/app-shell";
import { Avatar, useMe } from "@/components/me-context";
import { ProfilePhotoDialog } from "@/components/profile-photo-dialog";
import { SettingsCard, StatusDot } from "@/components/settings-frame";
import { MyActivity, formatThaiDate, getMyActivity } from "@/lib/api";
import { PERMISSIONS, can, roleLabel } from "@/lib/permissions";

const logoutUrl = process.env.NEXT_PUBLIC_LOGOUT_URL ?? "/oauth2/sign_out";

const actionTone: Record<string, "good" | "info" | "idle"> = { DOCUMENT_CONFIRMED: "good", DOCUMENT_UPDATED: "info", DOCUMENT_FILE_REPLACED: "info", DIRECTORY_USER_CREATED: "info" };

export default function ProfilePage() {
  const { me } = useMe();
  const [activity, setActivity] = useState<MyActivity | null>(null);
  const [error, setError] = useState("");
  const [photoOpen, setPhotoOpen] = useState(false);

  useEffect(() => {
    getMyActivity().then((response) => setActivity(response.data)).catch((reason: Error) => setError(reason.message));
  }, []);

  const role = me?.role ?? "";
  const details: [string, string][] = [["บัญชีผู้ใช้", me?.userId ?? "-"], ["อีเมล", me?.email || "ไม่พบในฐานข้อมูลบุคลากร"], ["หน่วยงาน", me?.department || "-"]];

  return <AppShell breadcrumb="หน้าหลัก / โปรไฟล์" title="โปรไฟล์ของฉัน" description="บัญชี สิทธิ์ และงานล่าสุดของคุณในระบบ">
    <div className="grid items-start gap-6 xl:grid-cols-3">
      <section className="card overflow-hidden">
        <div className="flex flex-col items-center gap-2.5 border-b border-[var(--line)] px-6 pb-6 pt-7 text-center">
          <span className="relative">
            <Avatar me={me} size={96} />
            <button type="button" aria-label="เปลี่ยนรูปโปรไฟล์" onClick={() => setPhotoOpen(true)} className="focus-ring absolute -bottom-0.5 -right-0.5 flex h-9 w-9 items-center justify-center rounded-full border border-[var(--line-strong)] bg-white shadow-[0_2px_6px_rgba(0,0,0,0.15)] hover:bg-[var(--canvas)]"><Camera size={17} /></button>
          </span>
          <div><p className="text-xl font-semibold text-[var(--navy)]">{me?.displayName ?? "กำลังโหลด..."}</p><p className="text-[13px] font-light text-[var(--muted)]">{me?.positionTitle || " "}</p></div>
          {role && <span className="rounded-full bg-[var(--accent-soft)] px-3 py-1 text-xs font-medium text-[#155e97]">{roleLabel(role)} · {role}</span>}
          <button type="button" onClick={() => setPhotoOpen(true)} className="focus-ring text-[13px] font-medium text-[var(--accent)] hover:underline">เปลี่ยนรูปโปรไฟล์</button>
        </div>
        <dl>{details.map(([label, value]) => <div key={label} className="border-b border-[var(--line-soft)] px-6 py-3.5"><dt className="text-xs text-[var(--muted)]">{label}</dt><dd className="mt-0.5 break-all text-sm font-medium">{value}</dd></div>)}
          <div className="px-6 py-3.5"><dt className="text-xs text-[var(--muted)]">เข้าสู่ระบบผ่าน</dt><dd className="mt-0.5 flex items-center gap-2 text-sm font-medium"><StatusDot tone="good" />SSO มหาวิทยาลัย</dd></div>
        </dl>
        <div className="space-y-3 border-t border-[var(--line)] bg-[var(--surface-muted)] px-6 py-4">
          <p className="text-xs font-light leading-6 text-[var(--muted)]">ข้อมูลบัญชีมาจากระบบของมหาวิทยาลัย แก้ไขชื่อหรืออีเมลได้ที่ระบบต้นทาง</p>
          <a href={logoutUrl} className="focus-ring flex h-11 items-center justify-center gap-2 rounded-md border border-[var(--line-strong)] bg-white text-sm hover:bg-[var(--line-soft)]"><SignOut size={17} />ออกจากระบบ</a>
        </div>
      </section>

      <div className="space-y-6 xl:col-span-2">
        {error && <div role="alert" className="rounded-lg bg-[var(--danger-soft)] p-4 text-sm font-medium text-[var(--danger)]">{error}</div>}
        <div role="status" className="card grid grid-cols-3">{[["นำเข้าเดือนนี้", activity?.stats.imported], ["ยืนยันเดือนนี้", activity?.stats.confirmed], ["แก้ไขเดือนนี้", activity?.stats.edited]].map(([label, value]) => <div key={String(label)} className="border-r border-[var(--line-soft)] px-5 py-4 last:border-0"><p className="text-xs text-[var(--muted)]">{label}</p><p className="mt-1 text-2xl font-semibold text-[var(--navy)]">{value ?? "-"}</p></div>)}</div>

        <SettingsCard title="งานล่าสุดของฉัน" description="จากประวัติการทำงานของคุณในระบบ">
          <ul>{(activity?.items ?? []).map((item) => {
            const title = item.documentTitle || item.details || "-";
            const content = <><span className="inline-flex w-28 shrink-0 items-center gap-1.5 text-xs text-[var(--muted)]"><StatusDot tone={actionTone[item.action] ?? "idle"} />{item.label}</span><span className="min-w-0 flex-1 truncate text-sm font-medium">{title}</span><span className="whitespace-nowrap text-xs text-[var(--subtle)]">{formatThaiDate(item.createdAt)}</span></>;
            return <li key={item.id} className="border-b border-[var(--line-soft)] last:border-0">{item.documentId ? <Link href={`/documents/${item.documentId}`} className="focus-ring flex items-center gap-3.5 px-6 py-3 hover:bg-[var(--brand-blue-pale)]">{content}</Link> : <div className="flex items-center gap-3.5 px-6 py-3">{content}</div>}</li>;
          })}{activity && activity.items.length === 0 && <li className="px-6 py-10 text-center text-sm text-[var(--muted)]">ยังไม่มีงานในระบบ</li>}</ul>
        </SettingsCard>

        <SettingsCard title="สิทธิ์ของฉัน" action={<Link href="/settings" className="text-[13px] text-[var(--accent)] hover:underline">ดูสิทธิ์ทุกบทบาท</Link>}>
          <ul className="grid gap-x-8 px-6 py-2 sm:grid-cols-2">{PERMISSIONS.map((permission) => {
            const allowed = can(role, permission.key);
            return <li key={permission.key} className="flex items-center gap-2.5 border-b border-[#f1f3f6] py-2.5 text-sm">{allowed ? <Check size={16} weight="bold" aria-label="มีสิทธิ์" className="shrink-0 text-[var(--success)]" /> : <span aria-label="ไม่มีสิทธิ์" className="w-4 shrink-0 text-center text-[#c9d1d9]">—</span>}<span className={allowed ? "" : "text-[var(--subtle)]"}>{permission.label}</span></li>;
          })}</ul>
        </SettingsCard>
      </div>
    </div>
    {photoOpen && <ProfilePhotoDialog onClose={() => setPhotoOpen(false)} />}
  </AppShell>;
}
