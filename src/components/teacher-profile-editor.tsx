"use client";

import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { ClockCounterClockwise, Plus, SpinnerGap, X } from "@phosphor-icons/react";
import { addDirectoryPersonPosition, getDirectoryPersonPositions, getDirectoryPersonRevisions, updateDirectoryUser } from "@/lib/api";
import type { DirectoryPersonRevision, DirectoryUser, PositionHolder } from "@/lib/api";
import { linkedRowsNote } from "@/lib/position-holder";
import { useToast } from "@/components/toast";

type Props = { user: DirectoryUser; onCancel: () => void; onSaved: () => void };
type ProfileDraft = Pick<DirectoryUser, "prefix" | "firstName" | "lastName" | "positionTitle" | "department" | "email" | "phone">;
type PositionDraft = { positionTitle: string; startsOn: string; endsOn: string };

const inputClass = "focus-ring h-10 w-full rounded-md border border-[var(--line-strong)] bg-white px-3 text-sm outline-none focus:border-[var(--accent)]";
const buttonClass = "focus-ring inline-flex min-h-10 items-center justify-center gap-2 rounded-md px-3 text-xs font-medium disabled:cursor-not-allowed disabled:opacity-50";
const fieldLabels: Record<string, string> = {
  prefix: "คำนำหน้า",
  firstName: "ชื่อ",
  lastName: "นามสกุล",
  positionTitle: "ตำแหน่ง",
  department: "หน่วยงาน",
  email: "อีเมล",
  phone: "เบอร์โทรศัพท์",
  positionTimeline: "ช่วงตำแหน่ง",
};

function initialDraft(user: DirectoryUser): ProfileDraft {
  return {
    prefix: user.prefix ?? "",
    firstName: user.firstName ?? "",
    lastName: user.lastName ?? "",
    positionTitle: user.positionTitle || user.jobTitle || "",
    department: user.department ?? "",
    email: user.email ?? "",
    phone: user.phone ?? "",
  };
}

function dateOnly(value?: string | null) {
  return value ? value.slice(0, 10) : "";
}

function thaiDate(value?: string | null) {
  const raw = dateOnly(value);
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
  if (!match) return raw || "ไม่ระบุวันที่";
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return new Intl.DateTimeFormat("th-TH", { day: "numeric", month: "short", year: "numeric" }).format(date);
}

function revisionDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "" : date.toLocaleString("th-TH", { dateStyle: "medium", timeStyle: "short" });
}

function positionSourceLabel(source: string) {
  if (source === "SYNC") return "จากข้อมูล sync";
  if (source === "LEARNED") return "จากการเลือกในเอกสาร";
  return "เจ้าหน้าที่บันทึก";
}

function displayValue(value: string) {
  return value.trim() ? value : "(ว่าง)";
}

export function TeacherProfileEditor({ user, onCancel, onSaved }: Props) {
  const toast = useToast();
  const [draft, setDraft] = useState<ProfileDraft>(() => initialDraft(user));
  const [positionDraft, setPositionDraft] = useState<PositionDraft>({ positionTitle: "", startsOn: "", endsOn: "" });
  const [positions, setPositions] = useState<PositionHolder[] | null>(null);
  const [revisions, setRevisions] = useState<DirectoryPersonRevision[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [profileError, setProfileError] = useState("");
  const [positionError, setPositionError] = useState("");
  const [saving, setSaving] = useState(false);
  const [addingPosition, setAddingPosition] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setLoadError("");
    Promise.all([getDirectoryPersonPositions(user.id), getDirectoryPersonRevisions(user.id)])
      .then(([positionResponse, revisionResponse]) => {
        if (!active) return;
        setPositions(positionResponse.data);
        setRevisions(revisionResponse.data);
      })
      .catch((reason: unknown) => {
        if (active) setLoadError(reason instanceof Error ? reason.message : "โหลดประวัติอาจารย์ไม่สำเร็จ");
      })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [user.id, refreshKey]);

  const changeDraft = (key: keyof ProfileDraft, value: string) => setDraft((current) => ({ ...current, [key]: value }));
  const changePositionDraft = (key: keyof PositionDraft, value: string) => setPositionDraft((current) => ({ ...current, [key]: value }));

  const saveProfile = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (saving) return;
    setSaving(true);
    setProfileError("");
    try {
      await updateDirectoryUser(user.id, draft);
      toast("บันทึกข้อมูลอาจารย์แล้ว");
      onSaved();
    } catch (reason) {
      setProfileError(reason instanceof Error ? reason.message : "บันทึกข้อมูลอาจารย์ไม่สำเร็จ");
    } finally {
      setSaving(false);
    }
  };

  const addPosition = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (addingPosition) return;
    setAddingPosition(true);
    setPositionError("");
    try {
      const result = await addDirectoryPersonPosition(user.id, {
        positionTitle: positionDraft.positionTitle.trim(),
        startsOn: positionDraft.startsOn,
        ...(positionDraft.endsOn ? { endsOn: positionDraft.endsOn } : {}),
      });
      setPositionDraft({ positionTitle: "", startsOn: "", endsOn: "" });
      toast(`เพิ่มช่วงตำแหน่งแล้ว${linkedRowsNote(result.linked)}`);
      setRefreshKey((current) => current + 1);
    } catch (reason) {
      setPositionError(reason instanceof Error ? reason.message : "บันทึกช่วงตำแหน่งไม่สำเร็จ");
    } finally {
      setAddingPosition(false);
    }
  };

  return <section aria-label={`แก้ไขข้อมูล ${user.fullName}`} className="space-y-5 rounded-lg border border-[var(--line-strong)] bg-[var(--surface-muted)] p-4 sm:p-5">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h3 className="text-base font-semibold text-[var(--navy)]">แก้ไขข้อมูลอาจารย์</h3>
        <p className="mt-1 text-xs leading-5 text-[var(--muted)]">ค่าที่แก้จะคงอยู่ แม้มีการ sync ข้อมูลบุคลากรจากต้นทางครั้งถัดไป</p>
      </div>
      <button type="button" onClick={onCancel} aria-label="ปิดแบบแก้ไข" className={`${buttonClass} border border-[var(--line-strong)] bg-white`}><X size={16} />ปิด</button>
    </div>

    <form onSubmit={saveProfile} className="space-y-4" aria-busy={saving}>
      {profileError && <p role="alert" className="rounded-md bg-[var(--danger-soft)] px-3 py-2 text-sm text-[var(--danger)]">{profileError}</p>}
      <fieldset disabled={saving} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <label className="space-y-1 text-xs"><span className="block">คำนำหน้า</span><input value={draft.prefix} maxLength={50} onChange={(event) => changeDraft("prefix", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">ชื่อ <span className="text-[var(--danger)]">*</span></span><input required value={draft.firstName} maxLength={255} onChange={(event) => changeDraft("firstName", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">นามสกุล <span className="text-[var(--danger)]">*</span></span><input required value={draft.lastName} maxLength={255} onChange={(event) => changeDraft("lastName", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">ตำแหน่ง</span><input value={draft.positionTitle} maxLength={255} onChange={(event) => changeDraft("positionTitle", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">หน่วยงาน / สังกัด</span><input value={draft.department} maxLength={255} onChange={(event) => changeDraft("department", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">อีเมล</span><input type="email" value={draft.email} maxLength={255} onChange={(event) => changeDraft("email", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">เบอร์โทรศัพท์</span><input type="tel" value={draft.phone} maxLength={20} onChange={(event) => changeDraft("phone", event.target.value)} className={inputClass} /></label>
      </fieldset>
      <div className="flex justify-end border-t border-[var(--line)] pt-3">
        <button type="submit" disabled={saving} className={`${buttonClass} bg-[var(--accent)] px-4 text-white hover:bg-[var(--accent-strong)]`}>{saving ? <SpinnerGap size={16} className="animate-spin" /> : null}{saving ? "กำลังบันทึก..." : "บันทึกข้อมูล"}</button>
      </div>
    </form>

    <section className="space-y-3 border-t border-[var(--line)] pt-4" aria-labelledby={`position-timeline-${user.id}`}>
      <div>
        <h4 id={`position-timeline-${user.id}`} className="text-sm font-semibold text-[var(--navy)]">ตำแหน่งตามช่วงเวลา</h4>
        <p className="mt-1 text-xs leading-5 text-[var(--muted)]">เพิ่มตำแหน่งบริหารที่มีผู้ดำรงคนเดียวได้หลายช่วง ระบบใช้วันที่ออกคำสั่งเลือกผู้ดำรงตำแหน่งให้ตรงกับช่วงเวลา</p>
      </div>
      <form onSubmit={addPosition} className="grid gap-3 rounded-md border border-[var(--line)] bg-white p-3 sm:grid-cols-2 lg:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end" aria-busy={addingPosition}>
        {positionError && <p role="alert" className="rounded-md bg-[var(--danger-soft)] px-3 py-2 text-xs text-[var(--danger)] sm:col-span-2 lg:col-span-4">{positionError}</p>}
        <label className="space-y-1 text-xs"><span className="block">ตำแหน่ง</span><input required maxLength={255} value={positionDraft.positionTitle} onChange={(event) => changePositionDraft("positionTitle", event.target.value)} placeholder="เช่น คณบดี" className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">เริ่มดำรงตำแหน่ง</span><input required type="date" value={positionDraft.startsOn} onChange={(event) => changePositionDraft("startsOn", event.target.value)} className={inputClass} /></label>
        <label className="space-y-1 text-xs"><span className="block">สิ้นสุด (เว้นว่างถ้ายังดำรงอยู่)</span><input type="date" min={positionDraft.startsOn || undefined} value={positionDraft.endsOn} onChange={(event) => changePositionDraft("endsOn", event.target.value)} className={inputClass} /></label>
        <button type="submit" disabled={addingPosition} className={`${buttonClass} bg-[var(--accent)] px-4 text-white hover:bg-[var(--accent-strong)]`}>{addingPosition ? <SpinnerGap size={16} className="animate-spin" /> : <Plus size={16} weight="bold" />}{addingPosition ? "กำลังเพิ่ม..." : "เพิ่มช่วง"}</button>
        <p className="text-[11px] leading-5 text-[var(--muted)] sm:col-span-2 lg:col-span-4">เมื่อเพิ่มช่วงใหม่ของตำแหน่งเดิม ระบบจะปิดช่วงเดิมก่อนวันเริ่มใหม่หนึ่งวัน เพื่อไม่ให้ช่วงเวลาทับซ้อนกัน</p>
      </form>

      {loading ? <p role="status" className="flex items-center gap-2 py-2 text-xs text-[var(--muted)]"><SpinnerGap size={15} className="animate-spin" />กำลังโหลด timeline และประวัติ...</p>
        : loadError ? <p role="alert" className="text-xs text-[var(--danger)]">{loadError}</p>
          : <>
            {!positions?.length ? <p className="rounded-md border border-dashed border-[var(--line-strong)] bg-white px-3 py-4 text-xs text-[var(--muted)]">ยังไม่มีประวัติตำแหน่ง</p>
              : <ol className="space-y-2">{positions.map((position) => <li key={position.id} className="grid gap-2 rounded-md border border-[var(--line)] bg-white px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
                <div><p className="break-words text-sm font-medium text-[var(--navy)]">{position.positionTitle}</p><p className="mt-1 text-xs text-[var(--muted)]">{position.startsOn ? thaiDate(position.startsOn) : "ไม่ระบุวันเริ่ม"} – {position.endsOn ? thaiDate(position.endsOn) : "ปัจจุบัน"}</p></div>
                <span className="text-[11px] text-[var(--subtle)]">{positionSourceLabel(position.source)}</span>
              </li>)}</ol>}
            {positions?.some((position) => position.source === "SYNC") && <p className="text-[11px] leading-5 text-[var(--muted)]">ตำแหน่งที่มาจาก sync เป็นข้อมูลปัจจุบันจากต้นทาง ส่วนช่วงที่เจ้าหน้าที่บันทึกจะแทนข้อมูล sync เฉพาะวันที่ในช่วงนั้น</p>}
          </>}
    </section>

    <section className="space-y-3 border-t border-[var(--line)] pt-4" aria-labelledby={`teacher-history-${user.id}`}>
      <h4 id={`teacher-history-${user.id}`} className="flex items-center gap-2 text-sm font-semibold text-[var(--navy)]"><ClockCounterClockwise size={18} />ประวัติการแก้ไขข้อมูล</h4>
      {loading ? null : loadError ? null : !revisions?.length ? <p className="rounded-md border border-dashed border-[var(--line-strong)] bg-white px-3 py-4 text-xs text-[var(--muted)]">ยังไม่มีประวัติการแก้ไข</p>
        : <ol className="divide-y divide-[var(--line)] rounded-md border border-[var(--line)] bg-white">{revisions.slice(0, 10).map((revision) => <li key={revision.id} className="space-y-2 px-3 py-3">
          <div className="flex flex-wrap justify-between gap-x-4 gap-y-1 text-xs"><span className="font-medium text-[var(--navy)]">{revision.changedBy || "ไม่ทราบผู้แก้ไข"}</span><time className="text-[var(--muted)]">{revisionDate(revision.changedAt)}</time></div>
          <ul className="space-y-1">{revision.changes.map((change, index) => <li key={`${revision.id}-${change.field}-${index}`} className="break-words text-xs leading-5 text-[var(--muted)]"><span className="font-medium text-[var(--navy)]">{fieldLabels[change.field] || change.field}:</span> {displayValue(change.oldValue)} <span aria-hidden="true">→</span> {displayValue(change.newValue)}</li>)}</ul>
        </li>)}</ol>}
    </section>
  </section>;
}
