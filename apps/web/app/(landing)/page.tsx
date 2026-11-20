import type { Metadata } from "next";
import { MulticaLanding } from "@/features/landing/components/multica-landing";
import { RedirectIfAuthenticated } from "@/features/landing/components/redirect-if-authenticated";

export const metadata: Metadata = {
  title: {
    absolute: "Delegora: AI Agent Company",
  },
  description:
    "Source-available platform that turns coding agents into real teammates. Assign tasks, track progress, compound skills.",
  openGraph: {
    title: "Delegora: AI Agent Company",
    description:
      "Manage your human + agent workforce in one place.",
    url: "/",
  },
  alternates: {
    canonical: "/",
  },
};

export default function LandingPage() {
  return (
    <>
      <RedirectIfAuthenticated />
      <MulticaLanding />
    </>
  );
}
