import type { Metadata } from "next";
import { pageMetadata } from "@/features/landing/utils/page-metadata";
import { HomePage } from "@/features/landing/site/home-page";
import { getHomeDownload } from "@/features/landing/utils/home-download";

export const metadata: Metadata = {
  title: "Homepage",
  description:
    "Delegora — source-available platform that turns coding agents into real teammates. Assign tasks, track progress, compound skills.",
  openGraph: {
    title: "Delegora: AI Agent Company",
    description:
      "Manage your human + agent workforce in one place.",
    url: "/homepage",
  },
  alternates: {
    canonical: "/homepage",
  },
};

export default async function HomepagePage() {
  return <HomePage download={await getHomeDownload()} />;
}
