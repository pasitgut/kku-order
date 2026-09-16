import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "DocFlow | ระบบจัดการคำสั่งแต่งตั้ง",
  description: "ระบบจัดการและสกัดข้อมูลเอกสารคำสั่งแต่งตั้งสำหรับวิทยาลัยการคอมพิวเตอร์",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="th">
      <body>{children}</body>
    </html>
  );
}
