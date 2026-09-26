export type Offset = { x: number; y: number };

// Math.min/max และการหารคืนค่า -0 ได้ ซึ่งทำให้เทียบค่าแบบ deep-equal ผิด
const clean = (value: number) => value + 0;

// ขนาดที่ทำให้ด้านสั้นของรูปเต็มกรอบสี่เหลี่ยมพอดี
export function coverScale(width: number, height: number, viewport: number) {
  return Math.max(viewport / width, viewport / height);
}

// ไม่ให้ลากรูปจนเห็นพื้นหลังในกรอบ
export function clampOffset(width: number, height: number, viewport: number, zoom: number, offset: Offset): Offset {
  const scale = coverScale(width, height, viewport) * zoom;
  const slackX = Math.max(0, (width * scale - viewport) / 2);
  const slackY = Math.max(0, (height * scale - viewport) / 2);
  return { x: clean(Math.min(slackX, Math.max(-slackX, offset.x))), y: clean(Math.min(slackY, Math.max(-slackY, offset.y))) };
}

// ส่วนของรูปต้นฉบับ (หน่วยพิกเซล) ที่อยู่ในกรอบ ใช้ตัดรูปลง canvas
export function sourceRect(width: number, height: number, viewport: number, zoom: number, offset: Offset) {
  const scale = coverScale(width, height, viewport) * zoom;
  const left = (viewport - width * scale) / 2 + offset.x;
  const top = (viewport - height * scale) / 2 + offset.y;
  return { x: clean(-left / scale), y: clean(-top / scale), size: viewport / scale };
}
