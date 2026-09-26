import type { Metadata } from "next";
import { Prompt } from "next/font/google";
import { MeProvider } from "@/components/me-context";
import { ToastProvider } from "@/components/toast";
import "./globals.css";

const prompt = Prompt({
  subsets: ["thai", "latin"],
  weight: ["300", "400", "500", "600", "700"],
  variable: "--font-prompt",
  display: "swap",
});

export const metadata: Metadata = {
  title: "DocFlow | ระบบจัดการคำสั่งแต่งตั้ง",
  description: "ระบบจัดการและสกัดข้อมูลเอกสารคำสั่งแต่งตั้งสำหรับวิทยาลัยการคอมพิวเตอร์",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="th" className={prompt.variable}>
      <body>
        <ToastProvider><MeProvider>{children}</MeProvider></ToastProvider>
      </body>
    </html>
  );
}
