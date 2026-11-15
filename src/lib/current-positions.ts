const sourceLabels: Record<string, string> = {
  SYNC: "ระบบวิทยาลัย",
  MANUAL: "กรอกเอง",
  LEARNED: "จากการตรวจเอกสาร",
};

// sourceLabel บอกว่าข้อมูลผู้ดำรงตำแหน่งมาจากไหน ค่าที่ไม่รู้จักแสดงตามเดิม
export function sourceLabel(source: string) {
  return sourceLabels[source] ?? source;
}
