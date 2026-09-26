"use client";

import { ChangeEvent, PointerEvent, useEffect, useRef, useState } from "react";
import { Minus, Plus, Trash, X } from "@phosphor-icons/react";
import { Avatar, useMe } from "@/components/me-context";
import { useToast } from "@/components/toast";
import { deleteMyPhoto, formatFileSize, uploadMyPhoto } from "@/lib/api";
import { Offset, clampOffset, coverScale, sourceRect } from "@/lib/photo-crop";

const VIEWPORT = 280;
const OUTPUT_SIZE = 512;
const MAX_SOURCE_BYTES = 10 * 1024 * 1024;
type LoadedImage = { url: string; element: HTMLImageElement; name: string; size: number };

export function ProfilePhotoDialog({ onClose }: { onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const dragRef = useRef<{ pointerX: number; pointerY: number; start: Offset } | null>(null);
  const { me, update } = useMe();
  const toast = useToast();
  const [image, setImage] = useState<LoadedImage | null>(null);
  const [zoom, setZoom] = useState(1);
  const [offset, setOffset] = useState<Offset>({ x: 0, y: 0 });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (dialog && !dialog.open) dialog.showModal();
  }, []);
  useEffect(() => () => { if (image) URL.revokeObjectURL(image.url); }, [image]);

  const choose = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (!/^image\/(jpeg|png|webp)$/.test(file.type) || file.size > MAX_SOURCE_BYTES) {
      toast("เลือกรูป JPG, PNG หรือ WebP ขนาดไม่เกิน 10 MB", { kind: "error" });
      return;
    }
    const url = URL.createObjectURL(file);
    const element = new window.Image();
    element.onload = () => { setImage({ url, element, name: file.name, size: file.size }); setZoom(1); setOffset({ x: 0, y: 0 }); };
    element.onerror = () => { URL.revokeObjectURL(url); toast("เปิดรูปนี้ไม่ได้", { kind: "error" }); };
    element.src = url;
  };

  const natural = image ? { width: image.element.naturalWidth, height: image.element.naturalHeight } : null;
  const changeZoom = (value: number) => {
    setZoom(value);
    if (natural) setOffset((current) => clampOffset(natural.width, natural.height, VIEWPORT, value, current));
  };

  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (!image) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { pointerX: event.clientX, pointerY: event.clientY, start: offset };
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;
    if (!drag || !natural) return;
    setOffset(clampOffset(natural.width, natural.height, VIEWPORT, zoom, { x: drag.start.x + event.clientX - drag.pointerX, y: drag.start.y + event.clientY - drag.pointerY }));
  };

  // ตัดเฉพาะส่วนในวงกลมเป็นรูป 512x512 ก่อนส่ง ไฟล์จึงเล็กและไม่ติดข้อมูลตำแหน่งจากกล้อง
  const save = async () => {
    if (!image || !natural) return;
    setBusy(true);
    try {
      const canvas = document.createElement("canvas");
      canvas.width = OUTPUT_SIZE;
      canvas.height = OUTPUT_SIZE;
      const context = canvas.getContext("2d");
      if (!context) throw new Error("เบราว์เซอร์นี้ตัดรูปไม่ได้");
      const rect = sourceRect(natural.width, natural.height, VIEWPORT, zoom, offset);
      context.drawImage(image.element, rect.x, rect.y, rect.size, rect.size, 0, 0, OUTPUT_SIZE, OUTPUT_SIZE);
      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/jpeg", 0.9));
      if (!blob) throw new Error("แปลงรูปไม่สำเร็จ");
      const response = await uploadMyPhoto(blob);
      update({ hasPhoto: true, photoVersion: response.data.photoVersion });
      toast("บันทึกรูปโปรไฟล์แล้ว");
      onClose();
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "บันทึกรูปไม่สำเร็จ", { kind: "error" });
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await deleteMyPhoto();
      update({ hasPhoto: false });
      toast("ลบรูปโปรไฟล์แล้ว");
      onClose();
    } catch (reason) {
      toast(reason instanceof Error ? reason.message : "ลบรูปไม่สำเร็จ", { kind: "error" });
    } finally {
      setBusy(false);
    }
  };

  const scale = natural ? coverScale(natural.width, natural.height, VIEWPORT) * zoom : 1;
  const imageStyle = natural ? { width: natural.width * scale, height: natural.height * scale, left: (VIEWPORT - natural.width * scale) / 2 + offset.x, top: (VIEWPORT - natural.height * scale) / 2 + offset.y } : undefined;

  return <dialog ref={dialogRef} aria-labelledby="photo-title" onCancel={(event) => { event.preventDefault(); onClose(); }} className="m-auto w-[min(94vw,640px)] rounded-xl bg-white p-0 text-[var(--ink)] shadow-[0_20px_48px_rgba(0,0,0,0.3)] backdrop:bg-[rgba(3,20,69,0.62)]">
    <div className="flex items-center justify-between border-b border-[var(--line)] px-6 py-4">
      <div><h2 id="photo-title" className="text-lg font-semibold text-[var(--navy)]">เปลี่ยนรูปโปรไฟล์</h2><p className="text-[13px] font-light text-[var(--muted)]">ลากรูปเพื่อจัดตำแหน่ง แล้วปรับขนาดให้พอดีวงกลม</p></div>
      <button type="button" aria-label="ปิด" onClick={onClose} className="focus-ring flex h-10 w-10 items-center justify-center rounded-md text-[var(--muted)] hover:bg-[var(--line-soft)]"><X size={18} /></button>
    </div>
    <div className="flex flex-col gap-6 p-6 sm:flex-row">
      <div className="flex flex-col gap-3.5">
        <div aria-label="พื้นที่จัดรูป" onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={() => { dragRef.current = null; }} className={`relative touch-none overflow-hidden rounded-[10px] bg-[#cdd5de] ${image ? "cursor-grab active:cursor-grabbing" : ""}`} style={{ width: VIEWPORT, height: VIEWPORT }}>
          {/* eslint-disable-next-line @next/next/no-img-element -- ภาพจากไฟล์ในเครื่อง (blob URL) ใช้ next/image ไม่ได้ */}
          {image && <img src={image.url} alt="" draggable={false} className="absolute max-w-none select-none" style={imageStyle} />}
          {!image && <label className="absolute inset-0 flex cursor-pointer flex-col items-center justify-center gap-2 text-center text-[13px] text-[var(--muted)]"><input type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" onChange={choose} /><span className="rounded-md bg-white px-3 py-2 font-medium text-[var(--accent)]">เลือกรูป</span>JPG, PNG หรือ WebP</label>}
          <span className="pointer-events-none absolute left-5 top-5 h-[240px] w-[240px] rounded-full border-2 border-white shadow-[0_0_0_200px_rgba(3,20,69,0.45)]" />
        </div>
        <label className="flex items-center gap-3"><span className="sr-only">ขนาดรูป</span><Minus size={16} className="text-[var(--muted)]" /><input type="range" min={1} max={3} step={0.05} value={zoom} disabled={!image} onChange={(event) => changeZoom(Number(event.target.value))} className="flex-1 accent-[var(--accent)]" /><Plus size={16} className="text-[var(--muted)]" /></label>
      </div>
      <div className="flex flex-1 flex-col gap-5">
        <div><p className="mb-2.5 text-[13px] font-medium text-[var(--navy)]">รูปปัจจุบัน</p><div className="flex items-end gap-4"><Avatar me={me} size={88} /><Avatar me={me} size={34} /></div></div>
        {image && <div className="flex items-center gap-3 rounded-lg border border-[var(--line)] px-3.5 py-3"><span className="min-w-0 flex-1"><span className="block truncate text-[13px] font-medium">{image.name}</span><span className="block text-xs text-[var(--muted)]">{formatFileSize(image.size)}</span></span><label className="cursor-pointer text-[13px] font-medium text-[var(--accent)]">เลือกรูปอื่น<input type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" onChange={choose} /></label></div>}
        <p className="text-xs font-light leading-6 text-[var(--muted)]">ระบบตัดรูปเป็น 512 × 512 px ก่อนบันทึก<br />ถ้าไม่มีรูป ระบบจะแสดงอักษรย่อชื่อแทน</p>
        {me?.hasPhoto && <button type="button" disabled={busy} onClick={remove} className="focus-ring inline-flex h-9 items-center gap-1.5 self-start text-[13px] text-[var(--muted)] hover:text-[var(--danger)] disabled:opacity-50"><Trash size={15} />ลบรูปปัจจุบัน</button>}
      </div>
    </div>
    <div className="flex justify-end gap-2.5 border-t border-[var(--line)] bg-[var(--surface-muted)] px-6 py-4">
      <button type="button" onClick={onClose} className="focus-ring h-11 rounded-md px-4 text-sm text-[var(--muted)] hover:bg-[var(--line-soft)]">ยกเลิก</button>
      <button type="button" disabled={!image || busy} onClick={save} className="focus-ring h-11 rounded-md bg-[var(--accent)] px-5 text-sm font-medium text-white hover:bg-[var(--accent-strong)] disabled:cursor-not-allowed disabled:bg-[var(--line-strong)] disabled:text-[var(--muted)]">{busy ? "กำลังบันทึก..." : "บันทึกรูป"}</button>
    </div>
  </dialog>;
}
