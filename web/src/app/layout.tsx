import type { Metadata } from "next";
import "./globals.css";


export const metadata: Metadata = {
  title: "PRISM · Property Evidence Desk",
  description: "Source-traceable property intelligence for the Pune pilot.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
